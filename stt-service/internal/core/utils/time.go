package utils

import "time"

const StorageTimeLayout = "2006-01-02T15:04:05.000000000Z"

func FormatStorageTime(value time.Time) string {
	return value.UTC().Format(StorageTimeLayout)
}

func FormatOptionalStorageTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := FormatStorageTime(*value)
	return &formatted
}
