package workflow

import (
	"sync"

	"goteams-client/internal/executor"
)

// 适配器注册表把「CLI 类型 → 适配器工厂」的绑定从 orchestrator 内部搬到统一入口，
// 新增 CLI 时只需要在新包内实现适配器，再到 bootstrap 注册一行，
// 不必再在编排层增加变量、Setter 与分支。
var (
	adapterFactoriesMu sync.RWMutex
	adapterFactories   = make(map[string]func() executor.Adapter)
)

// RegisterAdapterFactory 注册某个 CLI 类型的适配器工厂。
// 由 bootstrap 在启动阶段统一调用；重复注册同一类型会覆盖旧值，便于测试注入。
func RegisterAdapterFactory(cliType string, factory func() executor.Adapter) {
	if cliType == "" || factory == nil {
		return
	}
	adapterFactoriesMu.Lock()
	defer adapterFactoriesMu.Unlock()
	adapterFactories[cliType] = factory
}

// newAdapter 按 CLI 类型创建适配器，未注册的类型返回 false。
func newAdapter(cliType string) (executor.Adapter, bool) {
	adapterFactoriesMu.RLock()
	factory := adapterFactories[cliType]
	adapterFactoriesMu.RUnlock()
	if factory == nil {
		return nil, false
	}
	adapter := factory()
	if adapter == nil {
		return nil, false
	}
	return adapter, true
}

// RegisteredCLITypes 返回已注册适配器的 CLI 类型，用于诊断与自检。
func RegisteredCLITypes() []string {
	adapterFactoriesMu.RLock()
	defer adapterFactoriesMu.RUnlock()
	types := make([]string, 0, len(adapterFactories))
	for cliType := range adapterFactories {
		types = append(types, cliType)
	}
	return types
}
