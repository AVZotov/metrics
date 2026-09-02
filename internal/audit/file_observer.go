package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

var _ Observer = (*fileObserver)(nil)

type fileObserver struct {
	mu   sync.Mutex
	file *os.File
	name string
}

func newFileObserver(filePath string) (*fileObserver, error) {
	dirPath := filepath.Dir(filePath)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &fileObserver{
		file: f,
		name: filePath,
	}, nil
}

func (f *fileObserver) Notify(event Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	_, err = f.file.Write(data)
	return err
}

func (f *fileObserver) Name() string {
	return f.name
}

func (f *fileObserver) Close() error {
	return f.file.Close()
}
