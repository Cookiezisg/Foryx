// Package modelclient is THE model→client resolution chain (scenario → ModelRef →
// credentials → built Client + pre-filled base Request). Every LLM consumer outside
// the chat loop (utility sifter / envfix / web summariser / bootstrap resolvers) goes
// through this one function: the chain was hand-rolled three times before, and all
// three copies miswired Factory.Build's second return (the resolved base URL) into
// Request.ModelID — sending "model": "<base url>" on the wire and silently killing
// the feature behind it (acceptance AC-26).
//
// Package modelclient 是唯一的 model→client 解析链（scenario → ModelRef → 凭证 →
// 造好的 Client + 预填 base Request）。chat loop 之外的所有 LLM 消费方（utility
// 精选 / envfix / web 摘要 / bootstrap resolvers）都走这一个函数：此前这条链被手抄
// 三份，三份全把 Factory.Build 的第二返回值（解析后的 base URL）误接到
// Request.ModelID——线缆上发出 "model": "<base url>"，静默杀死其背后的功能（AC-26）。
package modelclient

import (
	"context"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// CredsResolver is the minimal credential port (consumer-side slice of
// apikeydomain.KeyProvider).
//
// CredsResolver 是最小凭证端口（apikeydomain.KeyProvider 的消费方切片）。
type CredsResolver interface {
	ResolveCredentialsByID(ctx context.Context, apiKeyID string) (apikeydomain.Credentials, error)
}

// Resolve runs the chain for a scenario (+ optional override) and returns the ready
// client, a base Request with ModelID/Key/BaseURL/Options pre-filled (System/Messages
// are the caller's), and the resolved provider (for turn provenance).
//
// Resolve 为 scenario（+ 可选 override）跑链，返回即用 client、预填 ModelID/Key/
// BaseURL/Options 的 base Request（System/Messages 由调用方填）、解析出的 provider
// （回合溯源用）。
func Resolve(
	ctx context.Context,
	scenario string,
	override *modeldomain.ModelRef,
	picker modeldomain.ModelPicker,
	keys CredsResolver,
	factory *llminfra.Factory,
) (llminfra.Client, llminfra.Request, string, error) {
	ref, err := modeldomain.Resolve(ctx, scenario, override, picker)
	if err != nil {
		return nil, llminfra.Request{}, "", err
	}
	creds, err := keys.ResolveCredentialsByID(ctx, ref.APIKeyID)
	if err != nil {
		return nil, llminfra.Request{}, "", err
	}
	client, baseURL, err := factory.Build(llminfra.Config{
		Provider:  creds.Provider,
		APIFormat: creds.APIFormat,
		ModelID:   ref.ModelID,
		Key:       creds.Key,
		BaseURL:   creds.BaseURL,
	})
	if err != nil {
		return nil, llminfra.Request{}, "", err
	}
	req := llminfra.Request{ModelID: ref.ModelID, Key: creds.Key, BaseURL: baseURL, Options: ref.Options}
	return client, req, creds.Provider, nil
}
