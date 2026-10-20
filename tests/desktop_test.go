package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/caliniya/caf/platforms/desktop"
)

func TestBindingConstructionHasNoStartupSideEffects(t *testing.T) {
	app := desktop.NewApp(desktop.New())
	status := app.GetStatus()
	if status.Name != "CAF" || status.Ready || status.DataDir != "" || status.Native.Loaded {
		t.Fatal("construction must not open directories or load QQ", status)
	}
	if _, err := app.LoadNativeLibrary(); err == nil {
		t.Fatal("native load accepted before startup")
	}
}

func TestDesktopStartupInitializesServicesWithoutLoadingNative(t *testing.T) {
	data := filepath.Join(t.TempDir(), "CAF data")
	t.Setenv("CAF_DATA_DIR", data)
	app := desktop.NewApp(desktop.New())
	app.Startup(context.Background())
	status := app.GetStatus()
	if !status.Ready || status.DataDir != data || status.LastError != "" || status.Native.Loaded {
		t.Fatal("unexpected startup state", status)
	}
	if info, err := os.Stat(filepath.Join(data, "logs")); err != nil || !info.IsDir() {
		t.Fatal("startup did not create the data directory", err)
	}
	if _, err := os.Stat(filepath.Join(data, "natives")); !os.IsNotExist(err) {
		t.Fatal("startup must not extract a native library", err)
	}
}

func TestDesktopStartupReportsInvalidDataDirectory(t *testing.T) {
	data := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(data, []byte("keep intact"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CAF_DATA_DIR", data)
	app := desktop.NewApp(desktop.New())
	app.Startup(context.Background())
	status := app.GetStatus()
	if status.Ready || status.LastError == "" || status.Native.Loaded {
		t.Fatal("startup failure was lost", status)
	}
	if _, err := app.LoadNativeLibrary(); err == nil || err.Error() != status.LastError {
		t.Fatal("native loading should report the startup error", err)
	}
}
