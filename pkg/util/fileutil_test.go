package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateOrOpenFile_NewFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.bin")
	f, err := CreateOrOpenFile(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer f.Close()
	if _, err := os.Stat(tmp); os.IsNotExist(err) {
		t.Fatal("file was not created")
	}
}

func TestCreateOrOpenFile_ExistingFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "existing.bin")
	if err := os.WriteFile(tmp, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	f, err := CreateOrOpenFile(tmp)
	if err != nil {
		t.Fatalf("unexpected error opening existing file: %v", err)
	}
	f.Close()
}

func TestPreallocateFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "prealloc.bin")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := PreallocateFile(f, 1024*1024); err != nil {
		t.Fatalf("PreallocateFile failed: %v", err)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 1024*1024 {
		t.Errorf("expected size 1048576, got %d", info.Size())
	}
}

func TestSyncAndClose(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "sync.bin")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("data"); err != nil {
		t.Fatal(err)
	}
	if err := SyncAndClose(f); err != nil {
		t.Fatalf("SyncAndClose failed: %v", err)
	}
}

func TestNewTransport_NoProxy(t *testing.T) {
	tr := NewTransport(4, 4*1024*1024, false, "")
	if tr == nil {
		t.Fatal("expected non-nil transport")
	}
	if tr.Proxy != nil {
		t.Error("expected nil Proxy when useProxy=false and proxyURL empty")
	}
}

func TestNewTransport_EnvProxy(t *testing.T) {
	tr := NewTransport(4, 4*1024*1024, true, "")
	if tr == nil {
		t.Fatal("expected non-nil transport")
	}
	if tr.Proxy == nil {
		t.Error("expected non-nil Proxy when useProxy=true")
	}
}

func TestNewTransport_ManualProxy(t *testing.T) {
	tr := NewTransport(4, 4*1024*1024, false, "http://proxy.example.com:8080")
	if tr == nil {
		t.Fatal("expected non-nil transport")
	}
	if tr.Proxy == nil {
		t.Error("expected non-nil Proxy when proxyURL is set")
	}
}

func TestNewTransport_InvalidProxyURL(t *testing.T) {
	// Invalid URL should fall through without panicking.
	tr := NewTransport(4, 4*1024*1024, false, "://bad-url")
	if tr == nil {
		t.Fatal("expected non-nil transport")
	}
}
