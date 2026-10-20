package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/caliniya/caf/core/platform"
	"github.com/caliniya/caf/platforms/desktop"
)

func TestDesktopBackendRequiresInitAndDoesNotLoadInConstructor(t *testing.T) {
	b := desktop.New()
	if b.NativeStatus().Loaded {
		t.Fatal("constructor loaded DLL")
	}
	if _, err := b.LoadNative(); !errors.Is(err, platform.ErrNotInitialized) {
		t.Fatal(err)
	}
	if err := b.BeginLogin(context.Background()); !errors.Is(err, platform.ErrNotInitialized) {
		t.Fatal(err)
	}
}

func TestDesktopPathsCannotChangeAfterInitialization(t *testing.T) {
	p := testPaths(t)
	b := desktop.New()
	if err := b.Init(p); err != nil {
		t.Fatal(err)
	}
	if err := b.Init(p); err != nil {
		t.Fatal(err)
	}
	if err := b.Init(testPaths(t)); !errors.Is(err, platform.ErrAlreadyInitialized) {
		t.Fatal(err)
	}
	if b.NativeStatus().Loaded {
		t.Fatal("initialization loaded DLL")
	}
}

func TestDesktopLoginExplicitlyReportsNotImplementedWithoutLoading(t *testing.T) {
	p := testPaths(t)
	b := desktop.New()
	if err := b.Init(p); err != nil {
		t.Fatal(err)
	}
	if err := b.BeginLogin(context.Background()); !errors.Is(err, platform.ErrLoginNotImplemented) {
		t.Fatal(err)
	}
	if b.NativeStatus().Loaded {
		t.Fatal("unimplemented login loaded native library")
	}
	if _, err := os.Stat(filepath.Join(p.Data, "natives")); !os.IsNotExist(err) {
		t.Fatal("unimplemented login extracted DLL", err)
	}
}

func TestDesktopLoginValidatesContext(t *testing.T) {
	b := desktop.New()
	if err := b.Init(testPaths(t)); err != nil {
		t.Fatal(err)
	}
	if err := b.BeginLogin(nil); !errors.Is(err, platform.ErrContextRequired) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.BeginLogin(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestDesktopCloseRejectsNewOperations(t *testing.T) {
	b := desktop.New()
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Init(testPaths(t)); !errors.Is(err, platform.ErrClosed) {
		t.Fatal(err)
	}
	if _, err := b.LoadNative(); !errors.Is(err, platform.ErrClosed) {
		t.Fatal(err)
	}
	if err := b.BeginLogin(context.Background()); !errors.Is(err, platform.ErrClosed) {
		t.Fatal(err)
	}
}
