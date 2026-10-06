package integration_test

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	recordingsapp "github.com/janickiy/go-recorder/internal/app/recordings"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	conferenceusecase "github.com/janickiy/go-recorder/internal/usecase/conferences"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
	"github.com/janickiy/go-recorder/internal/usecase/recordings"
	"gorm.io/gorm"
)

// Opt-in measurements run real production read paths against a randomly named,
// local-only database, created and dropped by stageOneDatabase. No app DB is used.
func TestP1DBWorkloads(t *testing.T) {
	if os.Getenv("RECORDER_P1_DB_PERF") != "true" {
		t.Skip("set RECORDER_P1_DB_PERF=true")
	}
	dir := os.Getenv("RECORDER_P1_DB_PROFILE_DIR")
	if dir == "" {
		t.Fatal("RECORDER_P1_DB_PROFILE_DIR is required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	count := 1000
	if raw := os.Getenv("RECORDER_P1_DB_CONFERENCES"); raw != "" {
		var err error
		count, err = strconv.Atoi(raw)
		if err != nil || count < 20 || count > 5000 {
			t.Fatal("conference count must be 20..5000")
		}
	}
	db := stageOneDatabase(t)
	seedP1DB(t, db, count)
	if os.Getenv("RECORDER_P1_DB_CANDIDATE_INDEX") == "true" {
		started := time.Now()
		if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_record_platform_page ON record(platform_conference_id, created_at DESC, id DESC) WHERE deleted_at IS NULL AND mode IN ('composite','audio_only','individual_tracks','screen_focus')`).Error; err != nil {
			t.Fatal(err)
		}
		var size int64
		if err := db.Raw("SELECT pg_relation_size('idx_record_platform_page')").Scan(&size).Error; err != nil {
			t.Fatal(err)
		}
		p1WriteJSON(t, dir, "candidate-index.json", map[string]any{"build_ms": time.Since(started).Milliseconds(), "size_bytes": size, "recordings": count * 20})
	}
	sqlDB, _ := db.DB()
	var settings []struct{ Name, Setting string }
	if err := db.Raw("SELECT name,setting FROM pg_settings WHERE name IN ('max_connections','shared_buffers','work_mem','statement_timeout','lock_timeout','server_version') ORDER BY name").Scan(&settings).Error; err != nil {
		t.Fatal(err)
	}
	p1WriteJSON(t, dir, "settings.json", settings)

	var counter atomic.Int64
	var capture atomic.Bool
	var mu sync.Mutex
	var captured []string
	hook := func(tx *gorm.DB) {
		counter.Add(1)
		if capture.Load() {
			mu.Lock()
			captured = append(captured, tx.Dialector.Explain(tx.Statement.SQL.String(), tx.Statement.Vars...))
			mu.Unlock()
		}
	}
	for name, register := range map[string]func(string, func(*gorm.DB)) error{
		"query": db.Callback().Query().After("gorm:query").Register,
		"row":   db.Callback().Row().After("gorm:row").Register,
		"raw":   db.Callback().Raw().After("gorm:raw").Register,
	} {
		if err := register("p1-count-"+name, hook); err != nil {
			t.Fatal(err)
		}
	}

	uid, cid, rid := p1ID("user", 1), p1ID("conference", 1), p1ID("record", 1)
	cr := pg.NewConferenceRepository(db)
	cs := conferenceusecase.NewService(cr, pg.NewUserRepository(db), nil)
	rr := pg.NewRecordRepository(db)
	rs := recordings.NewConferenceService(pg.NewConferenceRecordingRepository(db), recorder.NewService(rr, nil, nil, nil), nil, nil, nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("authenticated_user_id", uid) })
	router.GET("/api/v1/conferences/:id/recordings", recordingsapp.NewHandler(rs).List)
	chat := pg.NewChatRepository(db)
	notifications := pg.NewNotificationRepository(db)
	content := pg.NewContentRepository(db)
	analytics := pg.NewAnalyticsRepository(db)
	platform := pg.NewPlatformRepository(db)
	type workload struct {
		name string
		run  func(context.Context) error
	}
	workloads := []workload{
		{"recordings_http_20", func(ctx context.Context) error {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/conferences/"+cid+"/recordings?limit=20", nil).WithContext(ctx))
			if response.Code != http.StatusOK {
				return fmt.Errorf("HTTP %d", response.Code)
			}
			return nil
		}},
		{"recordings_1", func(ctx context.Context) error {
			v, e := rs.List(ctx, uid, cid, 1, 0)
			if e == nil && len(v) != 1 {
				return fmt.Errorf("recordings rows=%d", len(v))
			}
			return e
		}},
		{"recordings_20", func(ctx context.Context) error {
			v, e := rs.List(ctx, uid, cid, 20, 0)
			if e == nil && len(v) != 20 {
				return fmt.Errorf("recordings rows=%d", len(v))
			}
			return e
		}},
		{"meetings_20", func(ctx context.Context) error {
			_, e := cs.Timeline(ctx, uid, conferences.TimelineQuery{View: "past", Scope: "all", Limit: 20})
			return e
		}},
		{"history", func(ctx context.Context) error { _, e := cs.History(ctx, uid, cid); return e }},
		{"participants_20", func(ctx context.Context) error { _, e := cs.Participants(ctx, uid, cid, 20, 0); return e }},
		{"chat_20", func(ctx context.Context) error { _, e := chat.List(ctx, uid, cid, "", 20); return e }},
		{"notifications_20", func(ctx context.Context) error { _, e := notifications.List(ctx, uid, "", 20); return e }},
		{"transcript_20", func(ctx context.Context) error { _, e := content.Segments(ctx, uid, cid, rid, 20, 0); return e }},
		{"summary", func(ctx context.Context) error { _, _, e := content.Summary(ctx, uid, cid, rid); return e }},
		{"analytics", func(ctx context.Context) error { _, e := analytics.Read(ctx, uid, cid); return e }},
		{"admin", func(ctx context.Context) error {
			allowed, e := platform.IsAdmin(ctx, uid)
			if e != nil {
				return e
			}
			if !allowed {
				return fmt.Errorf("not admin")
			}
			_, e = platform.Summary(ctx)
			return e
		}},
	}
	type measurement struct {
		Name                     string
		Samples                  int
		Queries                  int64
		P50MS, P95MS, P99MS      float64
		BytesPerOp, MallocsPerOp uint64
		PoolBefore, PoolAfter    sql.DBStats
	}
	results := []measurement{}
	for _, w := range workloads {
		for i := 0; i < 5; i++ {
			if err := w.run(context.Background()); err != nil {
				t.Fatalf("warm %s: %v", w.name, err)
			}
		}
		counter.Store(0)
		captured = nil
		capture.Store(true)
		if err := w.run(context.Background()); err != nil {
			t.Fatal(err)
		}
		capture.Store(false)
		queryCount := counter.Load()
		p1WriteJSON(t, dir, w.name+"-queries.json", captured)
		// Explain the exact SQL emitted by production code, once per unique query.
		var plans []map[string]any
		seen := map[string]bool{}
		for _, q := range captured {
			if seen[q] {
				continue
			}
			seen[q] = true
			var lines []string
			if err := db.Raw("EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) " + q).Scan(&lines).Error; err != nil {
				t.Fatalf("explain %s: %v", w.name, err)
			}
			plans = append(plans, map[string]any{"sql": q, "plan": lines})
		}
		p1WriteJSON(t, dir, w.name+"-plans.json", plans)
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		stats := sqlDB.Stats()
		times := make([]float64, 100)
		for i := range times {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			start := time.Now()
			err := w.run(ctx)
			times[i] = float64(time.Since(start).Nanoseconds()) / 1e6
			cancel()
			if err != nil {
				t.Fatal(w.name, err)
			}
		}
		runtime.ReadMemStats(&after)
		sort.Float64s(times)
		m := measurement{w.name, len(times), queryCount, times[49], times[94], times[98], (after.TotalAlloc - before.TotalAlloc) / 100, (after.Mallocs - before.Mallocs) / 100, stats, sqlDB.Stats()}
		results = append(results, m)
		t.Logf("%s queries=%d p50=%.3f p95=%.3f p99=%.3f ms bytes/op=%d", w.name, queryCount, m.P50MS, m.P95MS, m.P99MS, m.BytesPerOp)
	}
	p1WriteJSON(t, dir, "measurements.json", results)
	// Contended production pool: 32 clients, 5 page requests each. Samples exclude
	// setup; pool statistics count queueing in database/sql, not a guessed metric.
	poolBefore := sqlDB.Stats()
	poolTimes := make([]float64, 160)
	var next atomic.Int64
	var peakOpen, peakInUse atomic.Int64
	monitorDone := make(chan struct{})
	monitorStop := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-monitorStop:
				return
			case <-ticker.C:
				stat := sqlDB.Stats()
				if int64(stat.OpenConnections) > peakOpen.Load() {
					peakOpen.Store(int64(stat.OpenConnections))
				}
				if int64(stat.InUse) > peakInUse.Load() {
					peakInUse.Store(int64(stat.InUse))
				}
			}
		}
	}()
	var wg sync.WaitGroup
	start := make(chan struct{})
	var errors atomic.Int64
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 5; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				requestStarted := time.Now()
				if workloads[2].run(ctx) != nil {
					errors.Add(1)
				}
				poolTimes[next.Add(1)-1] = float64(time.Since(requestStarted).Nanoseconds()) / 1e6
				cancel()
			}
		}()
	}
	started := time.Now()
	close(start)
	wg.Wait()
	close(monitorStop)
	<-monitorDone
	sort.Float64s(poolTimes)
	p1WriteJSON(t, dir, "pool-contention.json", map[string]any{"clients": 32, "requests": 160, "elapsed_ms": time.Since(started).Milliseconds(), "errors": errors.Load(), "before": poolBefore, "after": sqlDB.Stats(), "peak_open": peakOpen.Load(), "peak_in_use": peakInUse.Load(), "p50_ms": poolTimes[79], "p95_ms": poolTimes[151], "p99_ms": poolTimes[157]})
	if errors.Load() != 0 {
		t.Fatal("concurrent workload errors")
	}
	// Go profiles cover DB client/service work (the PostgreSQL server is separate).
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)
	defer runtime.SetMutexProfileFraction(0)
	defer runtime.SetBlockProfileRate(0)
	runtime.GC()
	for _, name := range []string{"heap", "allocs"} {
		f, e := os.Create(filepath.Join(dir, name+"-before-workload.pprof"))
		if e != nil {
			t.Fatal(e)
		}
		e = pprof.Lookup(name).WriteTo(f, 0)
		_ = f.Close()
		if e != nil {
			t.Fatal(e)
		}
	}
	cpu, err := os.Create(filepath.Join(dir, "cpu.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	if err = pprof.StartCPUProfile(cpu); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	profileCount := counter.Load()
	profileCycles := 0
	for time.Now().Before(deadline) {
		for _, w := range workloads {
			if err = w.run(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		profileCycles++
	}
	p1WriteJSON(t, dir, "profile-workload.json", map[string]any{"cycles": profileCycles, "operations": profileCycles * len(workloads), "queries": counter.Load() - profileCount})
	pprof.StopCPUProfile()
	_ = cpu.Close()
	runtime.GC()
	for _, name := range []string{"heap", "allocs", "mutex", "block", "goroutine"} {
		f, e := os.Create(filepath.Join(dir, name+".pprof"))
		if e != nil {
			t.Fatal(e)
		}
		e = pprof.Lookup(name).WriteTo(f, 0)
		_ = f.Close()
		if e != nil {
			t.Fatal(e)
		}
	}
	p1WriteJSON(t, dir, "dataset.json", map[string]any{"conferences": count, "users": 20, "participants": count * 20, "recordings": count * 20, "files": count * 40, "segments": count * 400, "events": count * 200, "chat_messages": count * 50, "notifications": count * 20, "transcripts": count * 20, "transcript_segments": count * 400, "summaries": count * 20, "go": runtime.Version(), "gomaxprocs": runtime.GOMAXPROCS(0), "pool_final": sqlDB.Stats(), "scope": "production usecase/repository reads; recordings_http_20 includes in-process Gin and JSON; excludes authentication, network/TLS and MinIO signing"})
	var sizes []struct {
		Name        string
		Rows, Bytes int64
	}
	if err := db.Raw(`SELECT relname AS name,n_live_tup AS rows,pg_total_relation_size(relid) AS bytes FROM pg_stat_user_tables ORDER BY relname`).Scan(&sizes).Error; err != nil {
		t.Fatal(err)
	}
	p1WriteJSON(t, dir, "relation-sizes.json", sizes)
}

func p1ID(kind string, n int) string {
	// Matches md5(kind||n)::uuid in fixture SQL without relying on random row order.
	// Hashing is only for deterministic synthetic identifiers, not authentication.
	return uuid.UUID(md5.Sum([]byte(kind + strconv.Itoa(n)))).String()
}

func p1WriteJSON(t *testing.T, dir, name string, value any) {
	t.Helper()
	data, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, name), data, 0600); e != nil {
		t.Fatal(e)
	}
}

func seedP1DB(t *testing.T, db *gorm.DB, n int) {
	t.Helper()
	queries := []string{
		`INSERT INTO users(id,email,password_hash,display_name,is_admin) SELECT md5('user'||i)::uuid,'p1-'||i||'@example.invalid','fixture-not-a-password','Participant '||i,i=1 FROM generate_series(1,20) i`,
		fmt.Sprintf(`INSERT INTO conferences(id,owner_id,title,invite_code,status,started_at,finished_at,created_at) SELECT md5('conference'||i)::uuid,md5('user1')::uuid,'Planning meeting '||i,md5('invite'||i),'finished',now()-interval '2 hours',now()-interval '1 hour',now()-interval '3 hours'+i*interval '1 millisecond' FROM generate_series(1,%d) i`, n),
		`INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,admission_state,joined_at,left_at) SELECT md5(c.id::text||u.id::text)::uuid,c.id,u.id,u.display_name,CASE WHEN u.is_admin THEN 'owner' ELSE 'participant' END,'left','admitted',now()-interval '2 hours',now()-interval '1 hour' FROM conferences c CROSS JOIN users u`,
		fmt.Sprintf(`INSERT INTO record(uuid,conference_id,platform_conference_id,requested_by,mode,source_type,transport_type,status,quality_mode,storage_object_key,duration_sec,size_bytes,metadata_json,created_at) SELECT md5('record'||i)::uuid,md5('conference'||((i-1)/20+1))::uuid,md5('conference'||((i-1)/20+1))::uuid,md5('user1')::uuid,'composite','conference','sfu','ready','auto','fixtures/'||i||'/final.mp4',3600,100000000,'{"fixture":true}',now()+i*interval '1 millisecond' FROM generate_series(1,%d) i`, n*20),
		`INSERT INTO record_file(record_id,file_type,bucket,object_key,mime_type) SELECT r.id,CASE WHEN i=1 THEN 'final_mp4' ELSE 'preview_jpg' END,'fixture','fixtures/'||r.uuid||'/'||i,CASE WHEN i=1 THEN 'video/mp4' ELSE 'image/jpeg' END FROM record r CROSS JOIN generate_series(1,2) i`,
		`INSERT INTO record_segment(record_id,seq_no,status,duration_ms,size_bytes) SELECT r.id,i,'uploaded',5000,100000 FROM record r CROSS JOIN generate_series(1,20) i`,
		`INSERT INTO record_event(record_id,event_type,event_source,message,payload_json) SELECT r.id,'segment.finished','recorder','Synthetic lifecycle event',jsonb_build_object('segment',i) FROM record r CROSS JOIN generate_series(1,10) i`,
		`INSERT INTO chat_messages(id,conference_id,sender_user_id,client_request_id,request_fingerprint,text) SELECT gen_random_uuid(),c.id,md5('user'||(i%20+1))::uuid,gen_random_uuid(),repeat('a',64),'Planning message number '||i FROM conferences c CROSS JOIN generate_series(1,50) i`,
		`INSERT INTO notifications(id,user_id,type,payload,dedup_key,read_at,published_at) SELECT gen_random_uuid(),u.id,'recording.ready',jsonb_build_object('conferenceId',c.id),'fixture:'||c.id,CASE WHEN c.id::text<'8' THEN now() ELSE NULL END,now() FROM conferences c CROSS JOIN users u`,
		`INSERT INTO transcripts(id,conference_id,recording_id,status) SELECT md5('transcript'||r.uuid)::uuid,r.conference_id,r.uuid,'ready' FROM record r`,
		`INSERT INTO transcript_segments(transcript_id,ordinal,start_ms,end_ms,text) SELECT t.id,i,i*1000,(i+1)*1000,'Synthetic transcript segment for planning and decisions '||i FROM transcripts t CROSS JOIN generate_series(1,20) i`,
		`INSERT INTO meeting_summaries(conference_id,transcript_id,transcript_generation,status,summary,structured_output) SELECT t.conference_id,t.id,1,'ready','Synthetic meeting summary','{"summary":"Synthetic meeting summary","keyPoints":["Decision"],"actionItems":[],"topics":[]}' FROM transcripts t`,
		`INSERT INTO conference_analytics(conference_id,duration_ms,participant_count,participant_timeline,audio_observation_enabled) SELECT id,3600000,20,'[{"atMs":0,"count":20}]',true FROM conferences`,
		`INSERT INTO participant_analytics(participant_id,conference_id,participation_ms,speaking_ms,observed_audio_ms,message_count) SELECT id,conference_id,3600000,90000,3600000,2 FROM conference_participants`,
		`ANALYZE`,
	}
	for i, q := range queries {
		if err := db.Exec(q).Error; err != nil {
			t.Fatalf("fixture query %d: %v", i, err)
		}
	}
}
