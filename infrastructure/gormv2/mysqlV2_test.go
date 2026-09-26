package gormv2

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"

	game "auxilia/domain/gamev2"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Optional real InnoDB run. Uses a fresh, uniquely named test database only.
// AUXILIA_V2_TEST_MYSQL_DSN needs CREATE/DROP DATABASE on a dedicated test server.
func mysqlTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AUXILIA_V2_TEST_MYSQL_DSN")
	if dsn == "" {
		return nil
	}
	cfg, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid test MySQL DSN")
	}
	cfg.DBName = ""
	cfg.ParseTime = true
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	name := "battle_v2_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4"); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	cfg.DBName = name
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestConcurrentCreateAndReplay(t *testing.T) {
	s, ids, room := testStore(t)
	ctx := context.Background()
	results := make(chan *View, 8)
	failures := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := s.Create(ctx, room, ids[0])
			if err != nil {
				failures <- err
			} else {
				results <- v
			}
		}()
	}
	wg.Wait()
	close(failures)
	close(results)
	for err := range failures {
		t.Fatal(err)
	}
	var id string
	for v := range results {
		if id != "" && id != v.Match.ID {
			t.Fatal("concurrent duplicate matches")
		}
		id = v.Match.ID
	}
	v := active(t, s, ids, room)
	command := game.Command{ID: "concurrent", ExpectedRevision: v.State.Revision}
	player := v.State.TurnPlayerID
	errorsCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Apply(ctx, id, player, "END_TURN", command); errorsCh <- err }()
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	current, err := s.Read(ctx, id, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if current.Match.LogSequence != v.Match.LogSequence+1 {
		t.Fatal("action committed more than once")
	}
}
