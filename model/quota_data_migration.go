package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// migrateQuotaDataBuckets prepares the schema and converts empty placeholders
// left by an interrupted upgrade to NULL before the unique index is created.
// Historical counters and identities are never recalculated, merged or replayed.
func migrateQuotaDataBuckets(db *gorm.DB) error {
	if !db.Migrator().HasTable(&QuotaData{}) {
		return nil
	}
	columns, err := db.Migrator().ColumnTypes(&QuotaData{})
	if err != nil {
		return err
	}
	hasBucketKey := false
	for _, column := range columns {
		switch column.Name() {
		case "bucket_key":
			hasBucketKey = true
			nullable, known := column.Nullable()
			_, hasDefault := column.DefaultValue()
			if (!known || nullable) && !hasDefault {
				continue
			}
			statement := &gorm.Statement{DB: db}
			if err := statement.Parse(&QuotaData{}); err != nil {
				return err
			}
			// The pinned PostgreSQL migrator does not remove an old default when
			// the target field has no default tag. An explicit DEFAULT NULL also
			// makes it repeat ALTERs on every restart. Drop the old constraints
			// once; an omitted column then defaults to NULL on every dialect.
			if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
				if err := db.Exec("ALTER TABLE ? ALTER COLUMN ? DROP NOT NULL, ALTER COLUMN ? DROP DEFAULT",
					clause.Table{Name: statement.Table}, clause.Column{Name: "bucket_key"}, clause.Column{Name: "bucket_key"}).Error; err != nil {
					return err
				}
			} else if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
				// SQLite rebuilds the table to relax a column constraint. Its
				// migrator drops standalone indexes/triggers, including objects
				// outside GORM tags, so restore them in the same transaction.
				if err := db.Transaction(func(tx *gorm.DB) error {
					var definitions []string
					if err := tx.Table("sqlite_master").Where("tbl_name = ? AND type IN ? AND sql IS NOT NULL", statement.Table, []string{"index", "trigger"}).Pluck("sql", &definitions).Error; err != nil {
						return err
					}
					if err := tx.Migrator().AlterColumn(&QuotaData{}, "BucketKey"); err != nil {
						return err
					}
					for _, definition := range definitions {
						if err := tx.Exec(definition).Error; err != nil {
							return err
						}
					}
					return nil
				}); err != nil {
					return err
				}
			} else if err := db.Migrator().AlterColumn(&QuotaData{}, "BucketKey"); err != nil {
				return err
			}
		case "model_name":
			// AutoMigrate can leave legacy varchar(64) unchanged. SQLite text
			// already accepts longer names; widen other databases only once.
			if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
				continue
			}
			if length, ok := column.Length(); ok && length > 0 && length < 128 {
				if err := db.Migrator().AlterColumn(&QuotaData{}, "ModelName"); err != nil {
					return err
				}
			}
		}
	}
	if hasBucketKey && !db.Migrator().HasIndex(&QuotaData{}, "idx_quota_data_bucket") {
		// Dropping DEFAULT '' does not change existing placeholders. They
		// must become NULL before AutoMigrate can create the unique index.
		// LENGTH also avoids MySQL treating trailing spaces as an empty key.
		// Never clear nonempty keys: existing event receipts depend on them.
		// Once the index exists, normal restarts skip this data update.
		if err := db.Model(&QuotaData{}).Where("LENGTH(bucket_key) = 0").UpdateColumn("bucket_key", nil).Error; err != nil {
			return err
		}
	}
	return nil
}
