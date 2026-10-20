package main

import "testing"

func TestBindingConstructionHasNoStartupSideEffects(t *testing.T) {
	app := NewApp()
	status := app.GetStatus()
	if status.Name != "CAF" || status.Ready || status.DataDir != "" || status.Native.Loaded {
		t.Fatal("construction must not open directories or load QQ", status)
	}
	if _, err := app.LoadNativeLibrary(); err == nil {
		t.Fatal("native load accepted before startup")
	}
}
