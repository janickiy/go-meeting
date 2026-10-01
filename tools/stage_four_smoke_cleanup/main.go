// stage_four_smoke_cleanup removes only a validated, terminal local acceptance
// fixture. It is deliberately separate from production routes and migrations.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	s3 "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
)

type manifest struct {
	RunID        string   `json:"runId"`
	ConferenceID string   `json:"conferenceId"`
	UserIDs      []string `json:"userIds"`
}

func canonical(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func sql(ctx context.Context, query string) (string, error) {
	command := exec.CommandContext(ctx, "docker", "compose", "exec", "-T", "postgres", "sh", "-c", `exec psql -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"`)
	command.Stdin = strings.NewReader(query)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("fixture SQL operation failed: %s", strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: stage_four_smoke_cleanup /absolute/cleanup.json")
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	var fixture manifest
	if json.Unmarshal(raw, &fixture) != nil || !strings.HasPrefix(fixture.RunID, "stage4-docker-") || !canonical(strings.TrimPrefix(fixture.RunID, "stage4-docker-")) || len(fixture.UserIDs) > 3 {
		return fmt.Errorf("invalid smoke fixture manifest")
	}
	if fixture.ConferenceID != "" && !canonical(fixture.ConferenceID) {
		return fmt.Errorf("invalid conference UUID")
	}
	ids := []string{}
	for _, id := range fixture.UserIDs {
		if !canonical(id) {
			return fmt.Errorf("invalid user UUID")
		}
		ids = append(ids, "'"+id+"'")
	}
	if len(ids) == 0 && fixture.ConferenceID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	userSet := strings.Join(ids, ",")
	if userSet != "" {
		invalid, err := sql(ctx, "SELECT count(*) FROM users WHERE id IN ("+userSet+") AND email NOT LIKE '"+fixture.RunID+"-%@smoke.invalid';")
		if err != nil {
			return err
		}
		if invalid != "0" {
			return fmt.Errorf("user identities are not this acceptance fixture")
		}
	}
	recordIDs := []string{}
	if fixture.ConferenceID != "" {
		conference, err := sql(ctx, "SELECT title FROM conferences WHERE id='"+fixture.ConferenceID+"';")
		if err != nil {
			return err
		}
		if conference != fixture.RunID {
			return fmt.Errorf("conference marker does not match this acceptance fixture")
		}
		active, err := sql(ctx, "SELECT count(*) FROM record WHERE conference_id='"+fixture.ConferenceID+"' AND (mode <> 'composite' OR status NOT IN ('ready','failed','cancelled'));")
		if err != nil {
			return err
		}
		if active != "0" {
			return fmt.Errorf("fixture has non-terminal or legacy recordings; cleanup refused")
		}
		values, err := sql(ctx, "SELECT uuid FROM record WHERE platform_conference_id='"+fixture.ConferenceID+"' AND mode='composite';")
		if err != nil {
			return err
		}
		for _, value := range strings.Fields(values) {
			if !canonical(value) {
				return fmt.Errorf("invalid record identity")
			}
			recordIDs = append(recordIDs, value)
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		origin := cfg.MinIOPublicOrigin
		if !strings.Contains(origin, "://") {
			origin = "http://" + origin
		}
		endpoint, err := url.Parse(origin)
		if err != nil || (endpoint.Hostname() != "localhost" && endpoint.Hostname() != "127.0.0.1" && endpoint.Hostname() != "::1") || endpoint.User != nil || endpoint.RawQuery != "" {
			return fmt.Errorf("cleanup requires a local MinIO public endpoint")
		}
		storage, err := s3.NewClient(ctx, endpoint.Host, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOBucket, endpoint.Scheme == "https")
		if err != nil {
			return err
		}
		if err = storage.RemovePrefix(ctx, "recordings/"+fixture.ConferenceID+"/"); err != nil {
			return err
		}
	}
	query := "BEGIN;\n"
	if fixture.ConferenceID != "" {
		query += "DELETE FROM conference_moderation_audit WHERE conference_id='" + fixture.ConferenceID + "';\n"
		query += "DELETE FROM record WHERE platform_conference_id='" + fixture.ConferenceID + "' AND mode='composite';\n"
		query += "DELETE FROM conferences WHERE id='" + fixture.ConferenceID + "' AND title='" + fixture.RunID + "';\n"
	}
	if userSet != "" {
		query += "DELETE FROM users WHERE id IN (" + userSet + ") AND email LIKE '" + fixture.RunID + "-%@smoke.invalid';\n"
	}
	query += "COMMIT;"
	if _, err = sql(ctx, query); err != nil {
		return err
	}
	for _, id := range recordIDs {
		// Every UUID came from the terminal records of the verified conference.
		path := filepath.Join("dockers", "storage", "data", "records", id)
		if err = os.RemoveAll(path); err != nil {
			return err
		}
	}
	fmt.Printf("Removed acceptance fixture: %d users, %d recordings; downloaded test artifacts retained.\n", len(ids), len(recordIDs))
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
