package integration_test

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Rehearses the forward migration against rows created with the previous schema.
func TestConferenceChatMigrationPreservesHistory(t *testing.T) {
	dsn := os.Getenv("RECORDER_STAGE1_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set RECORDER_STAGE1_TEST_POSTGRES_DSN to a local PostgreSQL URL with CREATE DATABASE permission")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || (parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1") {
		t.Fatal("migration rehearsal requires a local PostgreSQL URL")
	}
	admin, err := pg.Connect(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin = admin.Session(&gorm.Session{Logger: logger.Discard})
	adminSQL, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminSQL.Close() })
	databaseName := "go_recorder_chat_rehearsal_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE DATABASE " + databaseName).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP DATABASE " + databaseName + " WITH (FORCE)").Error; err != nil {
			t.Errorf("drop migration rehearsal database: %v", err)
		}
	})
	parsed.Path = "/" + databaseName
	db, err := pg.Connect(parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	db = db.Session(&gorm.Session{Logger: logger.Discard})
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	fullMigrations := filepath.Join("..", "..", "database", "migrations")
	oldMigrations := t.TempDir()
	files, err := filepath.Glob(filepath.Join(fullMigrations, "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		name := filepath.Base(path)
		if name >= "000029" {
			continue
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(oldMigrations, name), contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := pg.RunMigrations(db, oldMigrations); err != nil {
		t.Fatal(err)
	}
	owner, meeting, participant, message, notice := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	attachment := uuid.NewString()
	if err := db.Exec(`INSERT INTO users(id,email,password_hash,display_name) VALUES(?::uuid,'legacy@example.test','test','Legacy')`, owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO conferences(id,owner_id,title,invite_code) VALUES(?::uuid,?::uuid,'Legacy room','abcdefghijklmnopqrstuvwxyz123456')`, meeting, owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,joined_at)
		VALUES(?::uuid,?::uuid,?::uuid,'Legacy','owner','joined',now())`, participant, meeting, owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO chat_messages(id,conference_id,sender_user_id,client_request_id,request_fingerprint,text)
		VALUES(?::uuid,?::uuid,?::uuid,gen_random_uuid(),repeat('a',64),'Historical text')`, message, meeting, owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO chat_attachments(id,conference_id,owner_user_id,client_request_id,filename,mime_type,size,status,message_id,expires_at)
		VALUES(?::uuid,?::uuid,?::uuid,gen_random_uuid(),'old.txt','text/plain',3,'attached',?::uuid,now()+interval '1 day')`, attachment, meeting, owner, message).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO notifications(id,user_id,type,payload,dedup_key)
		VALUES(?::uuid,?::uuid,'conference.soon',jsonb_build_object('conferenceId',?::uuid),'legacy-notice')`, notice, owner, meeting).Error; err != nil {
		t.Fatal(err)
	}
	tables := []string{"users", "conferences", "conference_participants", "chat_messages", "chat_attachments", "notifications", "background_jobs"}
	fingerprints := map[string]string{}
	for _, table := range tables {
		fingerprints[table] = legacyTableFingerprint(t, db, table)
	}
	if err := pg.RunMigrations(db, fullMigrations); err != nil {
		t.Fatal(err)
	}
	if err := pg.RunMigrations(db, fullMigrations); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		if got := legacyTableFingerprint(t, db, table); got != fingerprints[table] {
			t.Fatalf("migration changed historical %s rows", table)
		}
	}
	var description string
	if err := db.Table("conferences").Select("chat_description").Where("id=?", meeting).Scan(&description).Error; err != nil || description != "" {
		t.Fatalf("legacy room has incorrect default description: %q %v", description, err)
	}
	var preferences int64
	if err := db.Table("conference_chat_preferences").Where("conference_id=?", meeting).Count(&preferences).Error; err != nil || preferences != 0 {
		t.Fatalf("legacy room unexpectedly opted out members: %d %v", preferences, err)
	}
}

func legacyTableFingerprint(t *testing.T, db *gorm.DB, table string) string {
	t.Helper()
	var digest string
	// The new conference column is removed from the comparison; every old field remains covered.
	query := "SELECT md5(COALESCE(string_agg((to_jsonb(t)-'chat_description')::text, '|' ORDER BY id::text),'')) FROM " + table + " t"
	if err := db.Raw(query).Scan(&digest).Error; err != nil {
		t.Fatal(err)
	}
	return digest
}
