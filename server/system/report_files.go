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

func (s *ReportFileService) List(query ListQuery) (ListResult[ReportFile], error) {
	query, _, err := NormalizeListQuery(query, "modified_at", "desc", map[string]string{
		"path":        "path",
		"name":        "name",
		"size":        "size",
		"modified_at": "modified_at",
	})
	if err != nil {
		return ListResult[ReportFile]{}, err
	}
	if err := s.EnsureRoot(); err != nil {
		return ListResult[ReportFile]{}, err
	}
	files := []ReportFile{}
	err = filepath.WalkDir(s.root, func(path string, entry fs.DirEntry, err error) error {
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
		return ListResult[ReportFile]{}, err
	}
	files = filterReportFiles(files, query)
	sort.Slice(files, func(i, j int) bool {
		less := reportFileLess(files[i], files[j], query.Sort)
		if query.Order == "desc" {
			return !less && files[i].Path != files[j].Path
		}
		return less
	})
	total := len(files)
	start := ListOffset(query)
	if start >= len(files) {
		files = []ReportFile{}
	} else {
		end := start + query.PageSize
		if end > len(files) {
			end = len(files)
		}
		files = files[start:end]
	}
	return NewListResult(files, query, total), nil
}

func filterReportFiles(files []ReportFile, query ListQuery) []ReportFile {
	filtered := make([]ReportFile, 0, len(files))
	extension := strings.TrimSpace(query.Filters["extension"])
	folder := strings.Trim(strings.TrimSpace(query.Filters["folder"]), "/")
	for _, file := range files {
		if query.Keyword != "" {
			keyword := strings.ToLower(query.Keyword)
			if !strings.Contains(strings.ToLower(file.Path), keyword) && !strings.Contains(strings.ToLower(file.Name), keyword) {
				continue
			}
		}
		if extension != "" && !strings.EqualFold(filepath.Ext(file.Name), extension) {
			continue
		}
		if folder != "" && !strings.HasPrefix(file.Path, folder+"/") && file.Path != folder {
			continue
		}
		filtered = append(filtered, file)
	}
	return filtered
}

func reportFileLess(a, b ReportFile, field string) bool {
	switch field {
	case "path":
		return a.Path < b.Path
	case "name":
		if a.Name == b.Name {
			return a.Path < b.Path
		}
		return a.Name < b.Name
	case "size":
		return a.Size < b.Size || (a.Size == b.Size && a.Path < b.Path)
	default:
		return a.ModifiedAt < b.ModifiedAt || (a.ModifiedAt == b.ModifiedAt && a.Path < b.Path)
	}
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
