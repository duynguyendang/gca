package ooda

import (
	"context"
	"fmt"

	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/duynguyendang/meb"
)

type OODAConfig struct {
	StoreManager StoreManager
	PromptLoader PromptLoader
	Model        Model
	Policy       Policy
}

func NewOODAConfig(storeManager StoreManager, promptLoader PromptLoader, model Model) *OODAConfig {
	return &OODAConfig{
		StoreManager: storeManager,
		PromptLoader: promptLoader,
		Model:        model,
		Policy:       DefaultPolicy,
	}
}

func NewOODALoopFromConfig(config *OODAConfig) *GCALoop {
	observer := NewGraphObserver(config.StoreManager)
	orienter := NewGraphOrienter(config.StoreManager)
	decider := NewGraphDecider(config.StoreManager, config.PromptLoader)
	verifier := NewPolicyVerifier(config.Policy)
	actor := NewGeminiActor(config.Model)

	return NewGCALoop(observer, orienter, decider, verifier, actor)
}

type GCAStoreManager struct {
	stores map[string]*meb.MEBStore
}

func NewGCAStoreManager() *GCAStoreManager {
	return &GCAStoreManager{
		stores: make(map[string]*meb.MEBStore),
	}
}

func (m *GCAStoreManager) RegisterStore(projectID string, store *meb.MEBStore) {
	m.stores[projectID] = store
}

func (m *GCAStoreManager) GetStore(projectID string) (*meb.MEBStore, error) {
	store, ok := m.stores[projectID]
	if !ok {
		return nil, fmt.Errorf("store not found for project: %s", projectID)
	}
	return store, nil
}

type GCAPromptLoader struct{}

func NewGCAPromptLoader() *GCAPromptLoader {
	return &GCAPromptLoader{}
}

func (l *GCAPromptLoader) LoadPrompt(name string) (*prompts.Prompt, error) {
	return prompts.LoadPrompt(name)
}

func RunOODATask(ctx context.Context, loop *GCALoop, projectID, input string, task GCATask, symbolID string, data interface{}) (string, error) {
	frame := NewGCAFrame(projectID, input, task)
	frame.SymbolID = symbolID
	frame.Data = data

	resultFrame, err := loop.Run(ctx, frame)
	if err != nil {
		return "", err
	}

	if resultFrame.ExecError != nil {
		return "", resultFrame.ExecError
	}

	return resultFrame.Response, nil
}
