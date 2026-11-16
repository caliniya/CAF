package Tests

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	backend "github.com/caliniya/caf/core/Backend"
)

type otherTestServer struct{}

func (*otherTestServer) Init(context.Context) error  { return nil }
func (*otherTestServer) Start(context.Context) error { return nil }
func (*otherTestServer) Close(context.Context) error { return nil }

func TestQQServerRegistrationReturnsSameInstance(t *testing.T) {
	previous := backend.Platform
	t.Cleanup(func() { backend.Platform = previous })
	backend.Init()
	qq, err := backend.Platform.Servers.Get[*backend.QQServer](backend.QQServerName)
	if err != nil || qq != &backend.Platform.QQ {
		t.Fatalf("registry did not return the Core QQ instance: %p / %v", qq, err)
	}
	qq.ListenHost = "127.0.0.1"
	if backend.Platform.QQ.ListenHost != "127.0.0.1" {
		t.Fatal("registry copied the service instead of sharing it")
	}
	if got, err := backend.Platform.Servers.Get[*backend.QQServer]("missing"); err == nil || got != nil {
		t.Fatalf("missing service was accepted: %v / %v", got, err)
	}
	if got, err := backend.Platform.Servers.Get[*otherTestServer](backend.QQServerName); err == nil || got != nil {
		t.Fatalf("wrong service type was accepted: %v / %v", got, err)
	}
}

func TestQQServerLifecycleRestoresOnlyEnabledBindings(t *testing.T) {
	instance := useQQAdapterCore(t)
	owner, err := instance.CreateExtensionClient("Main", "main")
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := instance.CreateQQAdapter("Enabled", "token", "10001")
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := instance.CreateQQAdapter("Disabled", "token", "20002")
	if err != nil {
		t.Fatal(err)
	}
	unbound, err := instance.CreateQQAdapter("Unbound", "token", "30003")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := instance.SetExtensionClientQQ(owner.ID, enabled.ID, "enabled", true); err != nil {
		t.Fatal(err)
	}
	if _, err := instance.SetExtensionClientQQ(owner.ID, disabled.ID, "disabled", false); err != nil {
		t.Fatal(err)
	}
	instance.CloseQQAdapters()

	reloaded := &backend.Core{Paths: instance.Paths, FS: instance.FS, QQ: backend.QQServer{ListenHost: "127.0.0.1"}}
	var service backend.Server = backend.NewQQServer(reloaded)
	t.Cleanup(reloaded.CloseQQAdapters)
	if err := service.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, adapter := range []backend.QQAdapter{*enabled, *disabled, *unbound} {
		waitQQAdapter(t, reloaded, adapter.ID, "stopped", adapter.QQ)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := service.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitQQAdapter(t, reloaded, enabled.ID, "disconnected", "10001")
	waitQQAdapter(t, reloaded, disabled.ID, "stopped", "20002")
	waitQQAdapter(t, reloaded, unbound.ID, "stopped", "30003")
	connectReverseWS(t, enabled.Address, "10001", "token")
	waitQQAdapter(t, reloaded, enabled.ID, "connected", "10001")
	if err := service.Start(ctx); err != nil {
		t.Fatalf("repeated start failed: %v", err)
	}
	waitQQAdapter(t, reloaded, enabled.ID, "connected", "10001")
	cancel()
	waitQQAdapter(t, reloaded, enabled.ID, "stopped", "10001")
	if err := service.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.Close(context.Background()); err != nil {
		t.Fatalf("repeated close failed: %v", err)
	}
}

func TestQQServerCloseCleansUpEvenWithCancelledContext(t *testing.T) {
	instance := useQQAdapterCore(t)
	service := backend.NewQQServer(instance)
	adapter, err := instance.CreateQQAdapter("Bot", "token", "10001")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := instance.StartQQAdapter(adapter.ID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("close did not return context cancellation: %v", err)
	}
	waitQQAdapter(t, instance, adapter.ID, "stopped", "10001")
	endpoint, _ := url.Parse(adapter.Address)
	listener, err := net.Listen("tcp", endpoint.Host)
	if err != nil {
		t.Fatalf("close did not release listener: %v", err)
	}
	_ = listener.Close()
}

func TestQQServerInitAndStartReturnConfigurationErrors(t *testing.T) {
	instance := useQQAdapterCore(t)
	service := backend.NewQQServer(instance)
	if err := os.MkdirAll(filepath.Join(instance.Paths.Data, "qq"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instance.Paths.Data, "qq", "adapters.json"), []byte("bad JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.Init(context.Background()); err == nil {
		t.Fatal("init accepted corrupted adapter configuration")
	}
	if err := service.Start(context.Background()); err == nil {
		t.Fatal("start accepted corrupted adapter configuration")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.Init(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("init ignored cancelled context: %v", err)
	}
	if err := service.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("start ignored cancelled context: %v", err)
	}
	if err := (&backend.QQServer{}).Init(context.Background()); err == nil {
		t.Fatal("unbound service accepted initialization")
	}
}
