package Backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// Only these UI operations can reach an authenticated Core. In particular there is no
// dynamic lookup of arbitrary methods or client-selected receiver, file root or profile.
var profileActions = map[string]reflect.Value{
	"ProbeSavedModels":                reflect.ValueOf((*Core).ProbeSavedModels),
	"UpdateModelCredentials":          reflect.ValueOf((*Core).UpdateModelCredentials),
	"GetChatDefaults":                 reflect.ValueOf((*Core).GetChatDefaults),
	"SetChatDefaults":                 reflect.ValueOf((*Core).SetChatDefaults),
	"SetChatCompactionMode":           reflect.ValueOf((*Core).SetChatCompactionMode),
	"SetChatWebSearch":                reflect.ValueOf((*Core).SetChatWebSearch),
	"CompactChat":                     reflect.ValueOf((*Core).CompactChat),
	"GetPendingCompaction":            reflect.ValueOf((*Core).GetPendingCompaction),
	"SubmitCompactionDecision":        reflect.ValueOf((*Core).SubmitCompactionDecision),
	"GetClientExtensionChatConfig":    reflect.ValueOf((*Core).GetClientExtensionChatConfig),
	"UpdateClientExtensionChatConfig": reflect.ValueOf((*Core).UpdateClientExtensionChatConfig),
	"ListQQContexts":                  reflect.ValueOf((*Core).ListQQContexts),
	"ClearQQContext":                  reflect.ValueOf((*Core).ClearQQContext),
	"DeleteQQContext":                 reflect.ValueOf((*Core).DeleteQQContext),
	"GetChatMemorySettings":           reflect.ValueOf((*Core).GetChatMemorySettings),
	"GetWorkspaceMemorySettings":      reflect.ValueOf((*Core).GetWorkspaceMemorySettings),
	"GetMemoryLibraryDetails":         reflect.ValueOf((*Core).GetMemoryLibraryDetails),
	"CreateMemoryEntry":               reflect.ValueOf((*Core).CreateMemoryEntry),
	"UpdateMemoryEntry":               reflect.ValueOf((*Core).UpdateMemoryEntry),
	"DeleteMemoryEntry":               reflect.ValueOf((*Core).DeleteMemoryEntry),
	"SetChatMemoryLibraries":          reflect.ValueOf((*Core).SetChatMemoryLibraries),
	"CreateMemoryLibrary":             reflect.ValueOf((*Core).CreateMemoryLibrary),
	"RenameMemoryLibrary":             reflect.ValueOf((*Core).RenameMemoryLibrary),
	"DeleteMemoryLibrary":             reflect.ValueOf((*Core).DeleteMemoryLibrary),
	"SetMemoryLibraryScope":           reflect.ValueOf((*Core).SetMemoryLibraryScope),
	"ListClientExtensions":            reflect.ValueOf((*Core).ListClientExtensions),
	"CreateClientExtension":           reflect.ValueOf((*Core).CreateClientExtension),
	"GetClientExtensionParameters":    reflect.ValueOf((*Core).GetClientExtensionParameters),
	"UpdateClientExtensionParameters": reflect.ValueOf((*Core).UpdateClientExtensionParameters),
	"DeleteClientExtension":           reflect.ValueOf((*Core).DeleteClientExtension),
	"ListExtensionTypes":              reflect.ValueOf((*Core).ListExtensionTypes),
	"SetClientExtensionEnabled":       reflect.ValueOf((*Core).SetClientExtensionEnabled),
	"ListExtensionClients":            reflect.ValueOf((*Core).ListExtensionClients),
	"GetExtensionClient":              reflect.ValueOf((*Core).GetExtensionClient),
	"CreateExtensionClient":           reflect.ValueOf((*Core).CreateExtensionClient),
	"RenameExtensionClient":           reflect.ValueOf((*Core).RenameExtensionClient),
	"DeleteExtensionClient":           reflect.ValueOf((*Core).DeleteExtensionClient),
	"SetExtensionClientQQ":            reflect.ValueOf((*Core).SetExtensionClientQQ),
	"RemoveExtensionClientQQ":         reflect.ValueOf((*Core).RemoveExtensionClientQQ),
	"SelectDirectory":                 reflect.ValueOf((*Core).SelectDirectory),
	"ListWorkspaces":                  reflect.ValueOf((*Core).ListWorkspaces),
	"CreateWorkspace":                 reflect.ValueOf((*Core).CreateWorkspace),
	"RenameWorkspace":                 reflect.ValueOf((*Core).RenameWorkspace),
	"DeleteWorkspace":                 reflect.ValueOf((*Core).DeleteWorkspace),
	"DeleteWorkspaceAndChats":         reflect.ValueOf((*Core).DeleteWorkspaceAndChats),
	"SetChatWorkspace":                reflect.ValueOf((*Core).SetChatWorkspace),
	"LoadConfigs":                     reflect.ValueOf((*Core).LoadConfigs),
	"GetChatModelContextWindow":       reflect.ValueOf((*Core).GetChatModelContextWindow),
	"ProbeModels":                     reflect.ValueOf((*Core).ProbeModels),
	"AddModelConfig":                  reflect.ValueOf((*Core).AddModelConfig),
	"RefreshModelConfig":              reflect.ValueOf((*Core).RefreshModelConfig),
	"SelectConfig":                    reflect.ValueOf((*Core).SelectConfig),
	"SelectModel":                     reflect.ValueOf((*Core).SelectModel),
	"RenameConfig":                    reflect.ValueOf((*Core).RenameConfig),
	"DeleteConfig":                    reflect.ValueOf((*Core).DeleteConfig),
	"ListChats":                       reflect.ValueOf((*Core).ListChats),
	"CreateChat":                      reflect.ValueOf((*Core).CreateChat),
	"ListPersonas":                    reflect.ValueOf((*Core).ListPersonas),
	"GetChat":                         reflect.ValueOf((*Core).GetChat),
	"RenameChat":                      reflect.ValueOf((*Core).RenameChat),
	"DeleteChat":                      reflect.ValueOf((*Core).DeleteChat),
	"SetChatPersona":                  reflect.ValueOf((*Core).SetChatPersona),
	"UpdateChatModel":                 reflect.ValueOf((*Core).UpdateChatModel),
	"SetChatReasoningEffort":          reflect.ValueOf((*Core).SetChatReasoningEffort),
	"ListToolSets":                    reflect.ValueOf((*Core).ListToolSets),
	"SetChatToolConfig":               reflect.ValueOf((*Core).SetChatToolConfig),
	"SetChatBranch":                   reflect.ValueOf((*Core).SetChatBranch),
	"SetChatToolChangesUndone":        reflect.ValueOf((*Core).SetChatToolChangesUndone),
	"StopChatGeneration":              reflect.ValueOf((*Core).StopChatGeneration),
	"SendChatMessageWithAttachments":  reflect.ValueOf((*Core).SendChatMessageWithAttachments),
	"GetChatImage":                    reflect.ValueOf((*Core).GetChatImage),
	"SendChatMessage":                 reflect.ValueOf((*Core).SendChatMessage),
	"SubmitUserAnswer":                reflect.ValueOf((*Core).SubmitUserAnswer),
	"SubmitToolApproval":              reflect.ValueOf((*Core).SubmitToolApproval),
	"EditChatMessage":                 reflect.ValueOf((*Core).EditChatMessage),
	"RegenerateChatMessage":           reflect.ValueOf((*Core).RegenerateChatMessage),
}

func (a *Accounts) Call(ticket, method, arguments string) (json.RawMessage, error) {
	p := a.p
	p.mu.Lock()
	s, err := p.session(ticket, false)
	if err != nil {
		p.mu.Unlock()
		return nil, err
	}
	s.calls.Add(1)
	p.mu.Unlock()
	defer s.calls.Done()
	if s.core.internal != nil {
		switch method {
		case "AddModelConfig", "RefreshModelConfig", "UpdateModelCredentials", "RenameConfig", "DeleteConfig", "ProbeModels", "ProbeSavedModels":
			return nil, errors.New("只有主用户可以管理模型供应商")
		}
	}
	f, ok := profileActions[method]
	if !ok {
		return nil, errors.New("未知用户操作")
	}
	t := f.Type()
	var raw []json.RawMessage
	if err = json.Unmarshal([]byte(arguments), &raw); err != nil || len(raw) != t.NumIn()-1 {
		return nil, errors.New("参数数量或格式无效")
	}
	args := []reflect.Value{reflect.ValueOf(s.core)}
	for i, r := range raw {
		v := reflect.New(t.In(i + 1))
		if err = json.Unmarshal(r, v.Interface()); err != nil {
			return nil, fmt.Errorf("参数 %d 格式无效", i+1)
		}
		args = append(args, v.Elem())
	}
	out := f.Call(args)
	if n := len(out); n > 0 && t.Out(n-1) == reflect.TypeFor[error]() {
		if !out[n-1].IsNil() {
			return nil, out[n-1].Interface().(error)
		}
		out = out[:n-1]
	}
	result := json.RawMessage("null")
	if len(out) > 0 {
		b, e := json.Marshal(out[0].Interface())
		if e != nil {
			return nil, e
		}
		result = b
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err = p.session(ticket, false); err != nil {
		return nil, err
	}
	return result, nil
}
