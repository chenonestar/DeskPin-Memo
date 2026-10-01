package service

import "deskpinmemo/internal/store"

// 子任务（FR-108）：事项下挂一层检查项，界面显示完成进度 x/y。
// 约定：完成子任务不会自动完成父事项，完成父事项也不会改动子任务状态——行为可预期，由用户决定。
// 所有操作都可撤销（Ctrl+Z）。

// AddSubtask 追加一条子任务。
func (s *Service) AddSubtask(itemID, title string) (store.Subtask, error) {
	sub, err := s.st.AddSubtask(itemID, title)
	if err != nil {
		return sub, err
	}
	s.pushUndo("添加子任务", func() error { return s.st.PurgeSubtask(sub.ID) })
	s.changed()
	return sub, nil
}

// ToggleSubtask 勾选 / 取消勾选子任务。
func (s *Service) ToggleSubtask(id string, done bool) (store.Subtask, error) {
	before, err := s.st.GetSubtask(id)
	if err != nil {
		return before, err
	}
	sub, err := s.st.UpdateSubtask(id, nil, &done)
	if err != nil {
		return sub, err
	}
	was := before.Done
	s.pushUndo("子任务完成状态", func() error { _, err := s.st.UpdateSubtask(id, nil, &was); return err })
	s.changed()
	return sub, nil
}

// RenameSubtask 修改子任务标题。
func (s *Service) RenameSubtask(id, title string) (store.Subtask, error) {
	before, err := s.st.GetSubtask(id)
	if err != nil {
		return before, err
	}
	sub, err := s.st.UpdateSubtask(id, &title, nil)
	if err != nil {
		return sub, err
	}
	old := before.Title
	s.pushUndo("修改子任务", func() error { _, err := s.st.UpdateSubtask(id, &old, nil); return err })
	s.changed()
	return sub, nil
}

// DeleteSubtask 删除子任务（可撤销，不弹确认框，见 4.4）。
func (s *Service) DeleteSubtask(id string) error {
	if err := s.st.DeleteSubtask(id); err != nil {
		return err
	}
	s.pushUndo("删除子任务", func() error { return s.st.RestoreSubtask(id) })
	s.changed()
	return nil
}

// ReorderSubtasks 调整子任务顺序（orderedIDs 为期望的完整顺序）。
func (s *Service) ReorderSubtasks(itemID string, orderedIDs []string) error {
	it, err := s.st.GetItem(itemID)
	if err != nil {
		return err
	}
	var prev []string
	for _, sub := range it.Subtasks {
		prev = append(prev, sub.ID)
	}
	if err := s.st.ReorderSubtasks(itemID, orderedIDs); err != nil {
		return err
	}
	s.pushUndo("调整子任务顺序", func() error { return s.st.ReorderSubtasks(itemID, prev) })
	s.changed()
	return nil
}
