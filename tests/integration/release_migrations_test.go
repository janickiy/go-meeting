package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
)

// TestReleaseMigrationLedger проверяет реальную атомарность, суммы и запуск без DDL.
// @args t — тест с изолированной PostgreSQL БД, создаваемой общим помощником.
func TestReleaseMigrationLedger(t *testing.T) {
	db := stageOneDatabase(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "release_probe.up.sql")
	sql := "CREATE TABLE IF NOT EXISTS release_probe (id INTEGER PRIMARY KEY, runs INTEGER NOT NULL); INSERT INTO release_probe VALUES(1,1) ON CONFLICT(id) DO UPDATE SET runs=release_probe.runs+1;"
	if err := os.WriteFile(path, []byte(sql), 0600); err != nil {
		t.Fatal(err)
	}
	// Старое приложение уже создало таблицу: новый журнал обязан выполнить SQL, а не пометить его вслепую.
	if err := db.Exec("CREATE TABLE release_probe (id INTEGER PRIMARY KEY, runs INTEGER NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO release_probe VALUES(1,4)").Error; err != nil {
		t.Fatal(err)
	}
	if err := pg.RunMigrations(db, dir); err != nil {
		t.Fatal(err)
	}
	var runs int
	db.Raw("SELECT runs FROM release_probe WHERE id=1").Scan(&runs)
	if runs != 5 {
		t.Fatalf("legacy migration not executed: %d", runs)
	}
	var workers sync.WaitGroup
	for range 6 {
		workers.Go(func() {
			if err := pg.RunMigrations(db, dir); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	db.Raw("SELECT runs FROM release_probe WHERE id=1").Scan(&runs)
	if runs != 5 {
		t.Fatal("already applied SQL was replayed")
	}
	t.Setenv("AUTO_MIGRATE", "false")
	if err := pg.RunStartupMigrations(db, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(sql+" -- changed"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, run := range []func() error{func() error { return pg.RunMigrations(db, dir) }, func() error { return pg.RunStartupMigrations(db, dir) }} {
		if err := run(); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
			t.Fatalf("changed SQL accepted: %v", err)
		}
	}
	if err := os.WriteFile(path, []byte(sql), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "zz_pending.up.sql"), []byte("CREATE TABLE release_pending(id INTEGER);"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := pg.RunStartupMigrations(db, dir); err == nil {
		t.Fatal("unapplied SQL accepted")
	}
	var exists bool
	db.Raw("SELECT to_regclass('release_pending') IS NOT NULL").Scan(&exists)
	if exists {
		t.Fatal("AUTO_MIGRATE=false executed DDL")
	}
}

// TestReleaseMigrationFailureAndLock доказывает откат SQL и журнала, а также ожидание advisory-lock.
// @args t — контекст изолированной проверки PostgreSQL.
func TestReleaseMigrationFailureAndLock(t *testing.T) {
	db := stageOneDatabase(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "release_a.up.sql"), []byte("CREATE TABLE release_atomic(id INTEGER);"), 0600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "release_b.up.sql")
	if err := os.WriteFile(bad, []byte("SELECT missing_release_function();"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := pg.RunMigrations(db, dir); err == nil {
		t.Fatal("invalid migration passed")
	}
	var exists bool
	db.Raw("SELECT to_regclass('release_atomic') IS NOT NULL").Scan(&exists)
	if exists {
		t.Fatal("DDL survived failed transaction")
	}
	var count int64
	db.Table("release_schema_migrations").Where("name LIKE 'release_%'").Count(&count)
	if count != 0 {
		t.Fatal("failed migration wrote ledger")
	}
	if err := os.Remove(bad); err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if err := tx.Exec("SELECT pg_advisory_xact_lock(748239105)").Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- pg.RunMigrations(db, dir) }()
	select {
	case err := <-done:
		t.Fatalf("lock bypassed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("migration did not resume")
	}
}
