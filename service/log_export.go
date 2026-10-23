package service

import (
	"context"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
	"sync"
	"time"
)

const logExportLockTTL = time.Minute

// AcquireLogExportLock returns a context cancelled on lease loss. Every
// instance uses the same SQL authority, including when Redis is unavailable.
func AcquireLogExportLock(parent context.Context, userID int) (context.Context, func(), bool, error) {
	key, owner := fmt.Sprintf("log_export:%d", userID), uuid.NewString()
	opCtx, stop := context.WithTimeout(parent, 5*time.Second)
	acquired, err := model.AcquireResourceLease(opCtx, key, owner, time.Now().Add(logExportLockTTL).Unix())
	stop()
	if err != nil || !acquired {
		return nil, nil, false, err
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(logExportLockTTL / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				opCtx, stop := context.WithTimeout(ctx, 5*time.Second)
				err := model.RenewResourceLease(opCtx, key, owner, time.Now().Add(logExportLockTTL).Unix())
				stop()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	var once sync.Once
	release := func() {
		once.Do(func() {
			cancel()
			<-done
			opCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			if err := model.ReleaseResourceLease(opCtx, key, owner); err != nil {
				common.SysError(common.LogText("Failed to release log export lease: %v", err))
			}
		})
	}
	return ctx, release, true, nil
}
