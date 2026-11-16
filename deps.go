package sailor

import (
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
)

// clock abstracts time so timed behaviour can be driven by tests.
type clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// fileSystem is the subset of os used by the consumer.
type fileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
}

type realFS struct{}

func (realFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (realFS) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}
func (realFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }

// watchEvent is a filesystem change. Path is empty when the backend does not say.
type watchEvent struct {
	Path string
}

// watcher delivers filesystem changes for watched directories.
type watcher interface {
	Add(dir string) error
	Events() <-chan watchEvent
	Errors() <-chan error
	Close() error
}

type fsnotifyWatcher struct {
	w    *fsnotify.Watcher
	ev   chan watchEvent
	err  chan error
	done chan struct{}
}

func newFsnotifyWatcher() (watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	fw := &fsnotifyWatcher{
		w:    w,
		ev:   make(chan watchEvent),
		err:  make(chan error),
		done: make(chan struct{}),
	}
	go fw.pump()
	return fw, nil
}

// pump forwards only the operations the consumer reacts to (create, write,
// rename, chmod), which cover a Kubernetes symlink swap.
func (f *fsnotifyWatcher) pump() {
	defer close(f.ev)
	for {
		select {
		case <-f.done:
			return
		case e, ok := <-f.w.Events:
			if !ok {
				return
			}
			if e.Has(fsnotify.Create) || e.Has(fsnotify.Write) || e.Has(fsnotify.Rename) || e.Has(fsnotify.Chmod) {
				select {
				case f.ev <- watchEvent{Path: e.Name}:
				case <-f.done:
					return
				}
			}
		case err, ok := <-f.w.Errors:
			if !ok {
				return
			}
			select {
			case f.err <- err:
			case <-f.done:
				return
			}
		}
	}
}

func (f *fsnotifyWatcher) Add(dir string) error      { return f.w.Add(dir) }
func (f *fsnotifyWatcher) Events() <-chan watchEvent { return f.ev }
func (f *fsnotifyWatcher) Errors() <-chan error      { return f.err }
func (f *fsnotifyWatcher) Close() error {
	close(f.done)
	return f.w.Close()
}

// deps collects every external dependency of a consumer.
type deps struct {
	clock      clock
	fs         fileSystem
	getenv     func(string) string
	homeDir    func() (string, error)
	hostname   func() (string, error)
	newWatcher func() (watcher, error)
}

func defaultDeps() deps {
	return deps{
		clock:      realClock{},
		fs:         realFS{},
		getenv:     os.Getenv,
		homeDir:    os.UserHomeDir,
		hostname:   os.Hostname,
		newWatcher: newFsnotifyWatcher,
	}
}
