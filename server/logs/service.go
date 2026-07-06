package logs

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	timestampLayout = "2006-01-02 15:04:05 -07:00"
	retentionPeriod = 7 * 24 * time.Hour
)

var (
	ErrInvalidStatus = errors.New("invalid log status")
	logFilePattern   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.log$`)
)

type Record struct {
	Timestamp string `json:"timestamp"`
	Status    string `json:"status"`
	IP        string `json:"ip"`
	UserID    string `json:"user_id"`
	Content   string `json:"content"`
}

type Query struct {
	Start    time.Time
	End      time.Time
	Statuses map[string]bool
	Limit    int
	Cursor   string
}

type QueryResult struct {
	Logs       []Record `json:"logs"`
	NextCursor string   `json:"next_cursor"`
}

type Service struct {
	directory string
	location  *time.Location
	mu        sync.Mutex
	stop      chan struct{}
	done      chan struct{}
}

type RequestLogger struct {
	service *Service
	ip      string
	userID  string
}

type contextKey struct{}

func NewService(directory string, location *time.Location) (*Service, error) {
	if strings.TrimSpace(directory) == "" {
		directory = "log"
	}
	if location == nil {
		location = time.Local
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	s := &Service{directory: directory, location: location, stop: make(chan struct{}), done: make(chan struct{})}
	if err := s.Cleanup(time.Now()); err != nil {
		return nil, err
	}
	go s.retentionWorker()
	return s, nil
}

func (s *Service) Close() { close(s.stop); <-s.done }

func (s *Service) With(ip, userID string) *RequestLogger {
	return &RequestLogger{service: s, ip: strings.TrimSpace(ip), userID: strings.TrimSpace(userID)}
}

func (l *RequestLogger) Log(status, content string) error {
	if l == nil || l.service == nil {
		return errors.New("logger is unavailable")
	}
	return l.service.write(time.Now(), status, l.ip, l.userID, content)
}

func IntoContext(ctx context.Context, logger *RequestLogger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

func FromContext(ctx context.Context) *RequestLogger {
	logger, _ := ctx.Value(contextKey{}).(*RequestLogger)
	return logger
}

func (s *Service) Log(status, content string) error { return s.With("", "").Log(status, content) }

func (s *Service) write(now time.Time, status, ip, userID, content string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	if !validStatus(status) {
		return ErrInvalidStatus
	}
	now = now.In(s.location)
	record := Record{Timestamp: now.Format(timestampLayout), Status: status, IP: ip, UserID: userID, Content: content}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.directory, now.Format("2006-01-02")+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("write log: %w", err)
	}
	return file.Sync()
}

func (s *Service) Read(query Query) (*QueryResult, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Limit > 1000 {
		query.Limit = 1000
	}
	offset, err := decodeCursor(query.Cursor)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return nil, err
	}
	records := []Record{}
	for _, entry := range entries {
		if entry.IsDir() || !logFilePattern.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(s.directory, entry.Name())
		fileRecords, err := readRecords(path)
		if err != nil {
			return nil, err
		}
		for _, record := range fileRecords {
			timestamp, err := time.Parse(timestampLayout, record.Timestamp)
			if err != nil {
				continue
			}
			if timestamp.Before(query.Start) || timestamp.After(query.End) {
				continue
			}
			if len(query.Statuses) > 0 && !query.Statuses[record.Status] {
				continue
			}
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Timestamp > records[j].Timestamp })
	if offset > len(records) {
		return nil, errors.New("invalid cursor")
	}
	end := offset + query.Limit
	if end > len(records) {
		end = len(records)
	}
	result := &QueryResult{Logs: records[offset:end]}
	if end < len(records) {
		result.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(end)))
	}
	return result, nil
}

func (s *Service) Cleanup(now time.Time) error {
	cutoff := now.Add(-retentionPeriod)
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return fmt.Errorf("read log directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !logFilePattern.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(s.directory, entry.Name())
		records, err := readRecords(path)
		if err != nil {
			return err
		}
		kept := records[:0]
		for _, record := range records {
			timestamp, err := time.Parse(timestampLayout, record.Timestamp)
			if err == nil && !timestamp.Before(cutoff) {
				kept = append(kept, record)
			}
		}
		if len(kept) == 0 {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		if len(kept) != len(records) {
			if err := rewriteRecords(path, kept); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) retentionWorker() {
	defer close(s.done)
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			if err := s.Cleanup(now); err != nil {
				fmt.Fprintf(os.Stderr, "log cleanup failed: %v\n", err)
			}
		case <-s.stop:
			return
		}
	}
}

func readRecords(path string) ([]Record, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	records := []Record{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var record Record
		if json.Unmarshal(scanner.Bytes(), &record) == nil {
			records = append(records, record)
		}
	}
	return records, scanner.Err()
}

func rewriteRecords(path string, records []Record) error {
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			file.Close()
			os.Remove(temporary)
			return err
		}
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(temporary)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(temporary)
		return err
	}
	return os.Rename(temporary, path)
}

func validStatus(status string) bool {
	return status == "info" || status == "warning" || status == "error"
}

func ParseStatuses(value string) (map[string]bool, error) {
	statuses := map[string]bool{}
	if strings.TrimSpace(value) == "" {
		return statuses, nil
	}
	for _, value := range strings.Split(value, ",") {
		status := strings.ToLower(strings.TrimSpace(value))
		if !validStatus(status) {
			return nil, ErrInvalidStatus
		}
		statuses[status] = true
	}
	return statuses, nil
}

func decodeCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, errors.New("invalid cursor")
	}
	offset, err := strconv.Atoi(string(decoded))
	if err != nil || offset < 0 {
		return 0, errors.New("invalid cursor")
	}
	return offset, nil
}
