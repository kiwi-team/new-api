package model

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
)

// Each instance must keep this directory on persistent storage. The image's
// working directory is /data, which the supplied compose file mounts as a volume.
// No in-memory state is needed to replay files after a process restart.
func quotaDataSpoolDir() string {
	if dir := os.Getenv("QUOTA_DATA_SPOOL_DIR"); dir != "" {
		return filepath.Clean(dir)
	}
	return "quota-data-spool"
}

func syncQuotaDataSpoolDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

// spoolQuotaDataEvent accepts only an already validated, serialized event. A
// successful return means both its contents and published filename were synced.
// Unique filenames avoid overwriting a file another worker is about to delete.
func spoolQuotaDataEvent(payload []byte) error {
	dir := quotaDataSpoolDir()
	// The configured parent must exist on the persistent volume. Creating just
	// this directory lets us durably sync its parent even on the first write.
	if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := syncQuotaDataSpoolDir(filepath.Dir(dir)); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".pending-*")
	if err != nil {
		return err
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	_, err = f.Write(payload)
	if err == nil {
		err = f.Sync()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	// CreateTemp only reserves the temporary name, not any derived final name.
	// Use an independent UUID so a later temporary-name reuse cannot overwrite
	// an earlier accepted event that has not yet been replayed.
	published := filepath.Join(dir, uuid.NewString()+".json")
	if err := os.Rename(temporary, published); err != nil {
		return err
	}
	return syncQuotaDataSpoolDir(dir)
}

// replayQuotaDataSpool transfers a bounded batch to the SQL inbox. Removing a
// file only after SQL confirmation makes crashes and concurrent readers safe:
// the original event ID and payload are deduplicated by the existing inbox.
func replayQuotaDataSpool(ctx context.Context) (int, error) {
	dir := quotaDataSpoolDir()
	f, err := os.Open(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()
	count, bytes := 0, 0
	for count < quotaDataBatchSize && bytes < quotaDataBatchBytes {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		entries, err := f.ReadDir(quotaDataBatchSize)
		if err != nil && !errors.Is(err, io.EOF) {
			return count, err
		}
		if len(entries) == 0 {
			break
		}
		for _, entry := range entries {
			if count == quotaDataBatchSize || bytes >= quotaDataBatchBytes {
				break
			}
			if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			eventFile, err := os.Open(path)
			if errors.Is(err, os.ErrNotExist) {
				continue // Another consumer already completed this file.
			}
			if err != nil {
				return count, err
			}
			payload, err := io.ReadAll(io.LimitReader(eventFile, quotaDataMaxPayloadBytes+1))
			if err = errors.Join(err, eventFile.Close()); err != nil {
				return count, err
			}
			bytes += len(payload)
			var event QuotaDataEvent
			if len(payload) > quotaDataMaxPayloadBytes {
				err = fmt.Errorf("%w: spool payload too large", errInvalidQuotaDataEvent)
			} else if err = common.Unmarshal(payload, &event); err != nil {
				err = fmt.Errorf("%w: malformed spool JSON", errInvalidQuotaDataEvent)
			} else {
				err = event.validate()
			}
			if err == nil {
				err = persistQuotaDataEvent(ctx, event, string(payload))
			}
			if errors.Is(err, errInvalidQuotaDataEvent) {
				// Keep malformed/conflicting bytes for repair without blocking the queue.
				if renameErr := os.Rename(path, path+".quarantined"); renameErr != nil && !errors.Is(renameErr, os.ErrNotExist) {
					return count, renameErr
				}
				common.SysError(common.LogText("Quota data spool file quarantined: file=%s", entry.Name()))
			} else if err != nil {
				return count, err
			} else if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return count, err
			}
			if err := syncQuotaDataSpoolDir(dir); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}
