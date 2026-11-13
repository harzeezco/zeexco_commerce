package database

import "testing"

func TestEmbeddedMigrationHistoryAllowsIntentionalVersionGap(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}
	want := []int{1, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	if len(migrations) != len(want) {
		t.Fatalf("migration count = %d, want %d", len(migrations), len(want))
	}
	for index, version := range want {
		if migrations[index].version != version || migrations[index].name == "" {
			t.Fatalf("migration[%d] = %#v", index, migrations[index])
		}
	}
}
