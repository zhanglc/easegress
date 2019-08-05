package scheduler

import (
	"fmt"
	"sync"

	"github.com/megaease/easegateway/pkg/logger"
	"github.com/megaease/easegateway/pkg/object/httpproxy"
	"github.com/megaease/easegateway/pkg/object/httpserver"
	"github.com/megaease/easegateway/pkg/object/seckill"
	"github.com/megaease/easegateway/pkg/registry"
)

var unitNewFuncs = map[string]unitNewFunc{
	httpserver.Kind: newServerUnit,
	httpproxy.Kind:  newProxyUnit,
	seckill.Kind:    newSeckillUnit,
}

func (s specsInOrder) Len() int      { return len(s) }
func (s specsInOrder) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s specsInOrder) Less(i, j int) bool {
	return scoreOfSpec(s[i]) < scoreOfSpec(s[j])
}
func scoreOfSpec(spec registry.Spec) int {
	switch spec.GetKind() {
	case seckill.Kind:
		return 0
	case httpproxy.Kind:
		return 1
	case httpserver.Kind:
		return 2
	}

	logger.Errorf("BUG: unsupported spec kind: %s", spec.GetKind())
	return 0
}

type (
	specsInOrder []registry.Spec

	unitNewFunc func(spec registry.Spec,
		handlers *sync.Map, first bool) (unit, error)

	status interface {
		// In second
		InjectTimestamp(uint64)
	}

	unit interface {
		status() status
		reload(spec registry.Spec)
		close()
	}

	serverUnit struct {
		server  *httpserver.HTTPServer
		runtime *httpserver.Runtime
	}

	proxyUnit struct {
		name     string
		proxy    *httpproxy.HTTPProxy
		runtime  *httpproxy.Runtime
		handlers *sync.Map
	}

	seckillUnit struct {
		name     string
		seckill  *seckill.Seckill
		runtime  *seckill.Runtime
		handlers *sync.Map
	}
)

func newServerUnit(spec registry.Spec, handlers *sync.Map, first bool) (unit, error) {
	serverSpec, ok := spec.(*httpserver.Spec)
	if !ok {
		return nil, fmt.Errorf("want *httpserver.Spec, got %T", spec)
	}
	runtime := httpserver.NewRuntime(handlers)
	return &serverUnit{
		server:  httpserver.New(serverSpec, runtime),
		runtime: runtime,
	}, nil
}

func (su *serverUnit) status() status {
	return su.runtime.Status()
}

func (su *serverUnit) reload(spec registry.Spec) {
	serverSpec, ok := spec.(*httpserver.Spec)
	if !ok {
		logger.Errorf("BUG: want *httpserver.Spec, got %T", spec)
	}

	olderServer := su.server
	su.server = httpserver.New(serverSpec, su.runtime)
	olderServer.Close()
}

func (su *serverUnit) close() {
	su.server.Close()
	su.runtime.Close()
}

func newProxyUnit(spec registry.Spec, handlers *sync.Map, first bool) (unit, error) {
	proxySpec, ok := spec.(*httpproxy.Spec)
	if !ok {
		return nil, fmt.Errorf("want *httpproxy.Spec, got %T", spec)
	}
	runtime := httpproxy.NewRuntime()
	pu := &proxyUnit{
		name:     spec.GetName(),
		proxy:    httpproxy.New(proxySpec, runtime),
		runtime:  runtime,
		handlers: handlers,
	}
	handlers.Store(pu.name, pu.proxy)

	return pu, nil
}

func (pu *proxyUnit) status() status {
	return pu.runtime.Status()
}

func (pu *proxyUnit) reload(spec registry.Spec) {
	proxySpec, ok := spec.(*httpproxy.Spec)
	if !ok {
		logger.Errorf("BUG: want *httpproxy.Spec, got %T", spec)
	}

	olderProxy := pu.proxy
	pu.proxy = httpproxy.New(proxySpec, pu.runtime)
	pu.handlers.Store(pu.name, pu.proxy)
	olderProxy.Close()
}

func (pu *proxyUnit) close() {
	pu.handlers.Delete(pu.proxy)
	pu.proxy.Close()
	pu.runtime.Close()
}

func newSeckillUnit(spec registry.Spec, handlers *sync.Map, first bool) (unit, error) {
	seckillSpec, ok := spec.(*seckill.Spec)
	if !ok {
		return nil, fmt.Errorf("want *seckill.Spec, got %T", spec)
	}
	runtime := seckill.NewRuntime(handlers)
	su := &seckillUnit{
		name:     spec.GetName(),
		seckill:  seckill.New(seckillSpec, runtime, first),
		runtime:  runtime,
		handlers: handlers,
	}
	handlers.Store(su.name, su.seckill)

	return su, nil
}

func (su *seckillUnit) status() status {
	return su.runtime.Status()
}

func (su *seckillUnit) reload(spec registry.Spec) {
	seckillSpec, ok := spec.(*seckill.Spec)
	if !ok {
		logger.Errorf("BUG: want *seckill.Spec, got %T", spec)
	}

	olderSeckill := su.seckill
	su.seckill = seckill.New(seckillSpec, su.runtime, false /*syncLoading*/)
	su.handlers.Store(su.name, su.seckill)
	olderSeckill.Close()
}

func (su *seckillUnit) close() {
	su.handlers.Delete(su.seckill)
	su.seckill.Close()
	su.runtime.Close()
}