package service

import (
	"time"

	"deskpinmemo/internal/store"
)

const metaOverviewDate = "overview_date"

// Overview 是「每日概览」（FR-308）：今天已逾期的和今天还要做的事项。
type Overview struct {
	Date    string     `json:"date"`
	Overdue []ItemView `json:"overdue"`
	Today   []ItemView `json:"today"`
	Total   int        `json:"total"`
}

// Overview 汇总当前需要关注的事项：已过截止时间的未完成事项 + 今天稍后到期的事项。
func (s *Service) Overview() (Overview, error) {
	now, loc := s.now(), s.loc()
	y, m, d := now.In(loc).Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	items, err := s.st.ListItems(store.Filter{Status: store.StatusTodo})
	if err != nil {
		return Overview{}, err
	}
	var over, today []store.Item
	for _, it := range items {
		if it.DueAt == nil {
			continue
		}
		switch due := *it.DueAt; {
		case due < now.UnixMilli():
			over = append(over, it)
		case due < end.UnixMilli():
			today = append(today, it)
		}
	}
	store.SortItems(over, now, loc)
	store.SortItems(today, now, loc)
	ov := Overview{Date: start.Format("2006-01-02"),
		Overdue: s.decorateGroupNames(s.views(over)), Today: s.decorateGroupNames(s.views(today))}
	ov.Total = len(ov.Overdue) + len(ov.Today)
	return ov, nil
}

// ClaimDailyOverview 在启动时调用：当天首次调用且有内容需要展示时返回 true。
// 无论是否有内容，当天都只「认领」一次——避免白天新建了事项后又莫名弹出。
// 设置关闭、数据已加密未解锁（读不到内容）时返回 false，且不占用当天的名额。
func (s *Service) ClaimDailyOverview() (bool, error) {
	if !s.GetSettings().DailyOverview || s.st.Locked() {
		return false, nil
	}
	today := s.now().In(s.loc()).Format("2006-01-02")
	if last, _ := s.st.GetMeta(metaOverviewDate); last == today {
		return false, nil
	}
	if err := s.st.SetMeta(metaOverviewDate, today); err != nil {
		return false, err
	}
	ov, err := s.Overview()
	if err != nil {
		return false, err
	}
	return ov.Total > 0, nil
}
