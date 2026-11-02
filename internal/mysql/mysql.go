// Package mysql supervises an isolated MySQL server owned by one project.
package mysql

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	mysqldriver "github.com/go-sql-driver/mysql"
	"laraserve/internal/runtime"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Server struct {
	Socket string
	Dir    string
	Env    map[string]string
	done   chan struct{}
	err    error // published by closing done
	cancel context.CancelFunc
	once   sync.Once
}

func (s *Server) Done() <-chan struct{} { return s.done }

func (s *Server) Err() error {
	select {
	case <-s.done:
		return s.err
	default:
		return nil
	}
}

// Close allows MySQL to flush its data before removing ephemeral socket files.
func (s *Server) Close() {
	s.once.Do(func() {
		s.cancel()
		<-s.done
	})
}

// Start never opens a TCP port or reads the machine's MySQL configuration.
func Start(parent context.Context, base, stateDir string, env []string, appNames ...string) (*Server, error) {
	var err error
	base, err = filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	env = runtime.MySQLEnv(base, env)
	stateDir, err = filepath.Abs(stateDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(stateDir, 0o700); err != nil {
		return nil, err
	}
	lock := filepath.Join(stateDir, "running.lock")
	if err := os.Mkdir(lock, 0o700); err != nil {
		return nil, fmt.Errorf("MySQL state is locked (%s); stop the other instance first. After a crash, verify no mysqld uses this data before removing the lock: %w", lock, err)
	}
	started := false
	defer func() {
		if !started {
			_ = os.Remove(lock)
		}
	}()
	appName := "laraserve"
	if len(appNames) > 0 {
		appName = appNames[0]
	}
	databaseName, err := projectDatabaseName(stateDir, appName)
	if err != nil {
		return nil, err
	}
	password, err := projectPassword(filepath.Join(stateDir, "password"))
	if err != nil {
		return nil, err
	}
	// A short, private temporary directory avoids Unix socket path length limits.
	sockets, err := os.MkdirTemp("", "laraserve-mysql-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if !started {
			_ = os.RemoveAll(sockets)
		}
	}()
	socket := filepath.Join(sockets, "mysql.sock")
	data := filepath.Join(stateDir, "data")
	server := filepath.Join(base, "bin", "mysqld")
	common := []string{"--no-defaults", "--basedir=" + base, "--datadir=" + data}
	if os.Geteuid() == 0 {
		common = append(common, "--user=root")
	}
	initialized := filepath.Join(stateDir, "initialized")
	if _, err := os.Stat(initialized); os.IsNotExist(err) {
		initCtx, cancel := context.WithTimeout(parent, 2*time.Minute)
		cmd := exec.CommandContext(initCtx, server, append(common, "--initialize-insecure")...)
		cmd.Env = env
		out, initErr := cmd.CombinedOutput()
		cancel()
		if initErr != nil {
			return nil, fmt.Errorf("MySQL initialization failed: %w\n%s\nPartial data was kept in %s", initErr, out, data)
		}
		if err := os.WriteFile(initialized, []byte("ready\n"), 0o600); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	logPath := filepath.Join(stateDir, "server.log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	args := append(common, "--socket="+socket, "--pid-file="+filepath.Join(sockets, "mysql.pid"),
		"--skip-networking", "--mysqlx=OFF", "--skip-log-bin", "--innodb-buffer-pool-size=64M")
	cmd := exec.CommandContext(ctx, server, args...)
	cmd.Env, cmd.Stdout, cmd.Stderr = env, log, log
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 30 * time.Second
	if err := cmd.Start(); err != nil {
		cancel()
		log.Close()
		return nil, err
	}
	s := &Server{Socket: socket, Dir: stateDir, done: make(chan struct{}), cancel: cancel}
	go func() {
		s.err = cmd.Wait()
		log.Close()
		_ = os.RemoveAll(sockets)
		_ = os.Remove(lock)
		close(s.done)
	}()
	started = true
	readyCtx, readyCancel := context.WithTimeout(ctx, 45*time.Second)
	defer readyCancel()
	for {
		probeCtx, probeCancel := context.WithTimeout(readyCtx, 2*time.Second)
		err := queryDatabase(probeCtx, base, socket, env, "SELECT 1;\n")
		probeCancel()
		if err == nil {
			break
		}
		select {
		case <-s.done:
			s.Close()
			return nil, fmt.Errorf("MySQL exited before becoming ready: %v; see %s", s.Err(), logPath)
		case <-readyCtx.Done():
			s.Close()
			return nil, fmt.Errorf("MySQL startup timed out or was canceled: %w; see %s", readyCtx.Err(), logPath)
		case <-time.After(100 * time.Millisecond):
		}
	}
	// The socket directory is 0700. Only this OS user can access the local
	// bootstrap root account; Laravel gets a separate database-scoped user.
	sql := "CREATE DATABASE IF NOT EXISTS `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;\n" +
		"CREATE USER IF NOT EXISTS 'laraserve'@'localhost' IDENTIFIED BY '" + password + "';\n" +
		"ALTER USER 'laraserve'@'localhost' IDENTIFIED BY '" + password + "';\n" +
		"GRANT ALL PRIVILEGES ON `" + databaseName + "`.* TO 'laraserve'@'localhost';\n"
	if err := queryDatabase(readyCtx, base, socket, env, sql); err != nil {
		s.Close()
		return nil, fmt.Errorf("MySQL database setup failed: %w", err)
	}
	s.Env = map[string]string{
		"DB_CONNECTION": "mysql", "DB_URL": "", "DB_HOST": "localhost", "DB_PORT": "3306",
		"DB_SOCKET": socket, "DB_DATABASE": databaseName, "DB_USERNAME": "laraserve", "DB_PASSWORD": password,
	}
	return s, nil
}

// Use the native Go protocol client: older MySQL command-line clients depend
// on terminal libraries that are absent on many clean client machines.
var queryDatabase = query

func query(ctx context.Context, base, socket string, env []string, statement string) error {
	cfg := mysqldriver.NewConfig()
	cfg.User = "root"
	cfg.Net = "unix"
	cfg.Addr = socket
	cfg.MultiStatements = true
	cfg.Timeout = time.Second
	cfg.ReadTimeout = 2 * time.Second
	cfg.WriteTimeout = 2 * time.Second
	connector, err := mysqldriver.NewConnector(cfg)
	if err != nil {
		return err
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	_, err = db.ExecContext(ctx, statement)
	if err != nil {
		return fmt.Errorf("local MySQL query failed: %w", err)
	}
	return nil
}

func projectPassword(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		password := strings.TrimSpace(string(b))
		decoded, err := hex.DecodeString(password)
		if err != nil || len(decoded) != 32 {
			return "", fmt.Errorf("invalid managed MySQL password file %s", path)
		}
		return password, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	password := hex.EncodeToString(b)
	return password, os.WriteFile(path, []byte(password), 0o600)
}
