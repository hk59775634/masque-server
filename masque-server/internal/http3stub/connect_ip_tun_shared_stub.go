//go:build !linux

package http3stub

import (
	"context"

	"github.com/quic-go/quic-go/http3"
)

func runConnectIPSharedTunSessionLoop(ctx context.Context, str *http3.Stream, cfg ListenConfig, acl map[string]any) {
	runConnectIPDatagramEchoOnlyLoop(ctx, str, cfg, acl)
}
