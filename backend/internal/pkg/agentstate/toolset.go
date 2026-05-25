package agentstate

// ActivateGroup marks category as having been on-demand loaded this conversation; lazy-inits the map.
//
// ActivateGroup 记录 category 在本对话中已按需加载；懒初始化 map。
func (s *AgentState) ActivateGroup(cat string) {
	s.groupMu.Lock()
	defer s.groupMu.Unlock()
	if s.activatedGroups == nil {
		s.activatedGroups = make(map[string]bool)
	}
	s.activatedGroups[cat] = true
}

// ActivatedGroups returns a snapshot slice of categories activated this conversation.
//
// ActivatedGroups 返回本对话已激活的 category 快照切片。
func (s *AgentState) ActivatedGroups() []string {
	s.groupMu.Lock()
	defer s.groupMu.Unlock()
	if len(s.activatedGroups) == 0 {
		return nil
	}
	out := make([]string, 0, len(s.activatedGroups))
	for k := range s.activatedGroups {
		out = append(out, k)
	}
	return out
}
