package system

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	ErrReportFileInvalid = errors.New("invalid report file path")
	ErrReportFileMissing = errors.New("report file not found")
)

type ReportFile struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at"`
}

type ReportFileService struct {
	root string
}

func NewReportFileService(root string) *ReportFileService {
	if strings.TrimSpace(root) == "" {
		root = "report"
	}
	return &ReportFileService{root: root}
}

func (s *ReportFileService) EnsureRoot() error {
	return os.MkdirAll(s.root, 0o755)
}

func (s *ReportFileService) List() ([]ReportFile, error) {
	if err := s.EnsureRoot(); err != nil {
		return nil, err
	}
	files := []ReportFile{}
	err := filepath.WalkDir(s.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		files = append(files, ReportFile{
			Path:       filepath.ToSlash(relative),
			Name:       entry.Name(),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].ModifiedAt == files[j].ModifiedAt {
			return files[i].Path < files[j].Path
		}
		return files[i].ModifiedAt > files[j].ModifiedAt
	})
	return files, nil
}

func (s *ReportFileService) Path(relative string) (string, error) {
	if err := s.EnsureRoot(); err != nil {
		return "", err
	}
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(relative)))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrReportFileInvalid
	}
	path := filepath.Join(s.root, clean)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && info.IsDir()) {
		return "", ErrReportFileMissing
	}
	if err != nil {
		return "", err
	}
	return path, nil
}

func (s *ReportFileService) Delete(relative string) error {
	path, err := s.Path(relative)
	if err != nil {
		return err
	}
	return os.Remove(path)
}
