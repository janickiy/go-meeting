package integration_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRemovedHandAnalyticsSchema проверяет удаление колонок и обратимость структуры миграции.
// Изменения выполняются только в отдельной тестовой БД; пользовательские данные не затрагиваются.
// @args t — контекст теста и адресная очистка созданной базы.
func TestRemovedHandAnalyticsSchema(t *testing.T) {
	db := stageOneDatabase(t)
	assertColumns := func(expected int64) {
		t.Helper()
		var count int64
		err := db.Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='participant_analytics' AND column_name IN ('hand_raises','last_hand_at')").Scan(&count).Error
		if err != nil || count != expected {
			t.Fatalf("unexpected removed columns: count=%d err=%v", count, err)
		}
	}
	assertColumns(0)
	for _, direction := range []string{"down", "up"} {
		sql, err := os.ReadFile(filepath.Join("..", "..", "database", "migrations", "000022_remove_raised_hands."+direction+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(string(sql)).Error; err != nil {
			t.Fatal(err)
		}
		if direction == "down" {
			assertColumns(2)
		} else {
			assertColumns(0)
		}
	}
	var preserved int64
	if err := db.Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='participant_analytics' AND column_name IN ('participation_ms','speaking_ms','screen_ms','message_count')").Scan(&preserved).Error; err != nil || preserved != 4 {
		t.Fatalf("remaining analytics columns changed: %d err=%v", preserved, err)
	}
}
