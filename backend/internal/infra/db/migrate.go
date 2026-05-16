package db

import (
	"fmt"

	"gorm.io/gorm"
)

// Migrate runs AutoMigrate on models in order then applies schema extras; idempotent.
//
// Migrate 按顺序 AutoMigrate 给定 model 再应用 schema extras；幂等。
func Migrate(db *gorm.DB, models ...any) error {
	if db == nil {
		return fmt.Errorf("migrate: nil db")
	}
	for i, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return fmt.Errorf("migrate model #%d (%T): %w", i, m, err)
		}
	}
	return applySchemaExtras(db)
}
