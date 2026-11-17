package service

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"deskpinmemo/internal/store"
)

// 桌面宠物「戴眼镜的狗狗」（摸鱼功能 FR-801~810）。
// 经验只来自完成事项；不会因为逾期扣分，也不会「死亡」——它只是陪着你。

// PetMaxLevel 是最高等级。
const PetMaxLevel = 10

// PetExpPerItem 是完成一个事项获得的经验。
const PetExpPerItem = 10

// PetDefaultName 是默认名字。
const PetDefaultName = "博士"

// petUnlock：配饰 / 眼镜款式及解锁等级。
var (
	petAccessories = []struct {
		ID    string
		Level int
	}{{"", 1}, {"bow", 2}, {"scarf", 4}, {"hat", 6}, {"crown", 9}}
	petGlasses = []struct {
		ID    string
		Level int
	}{{"round", 1}, {"square", 3}, {"sunglasses", 5}, {"gold", 7}}
)

// PetLevelThreshold 返回达到 level 级所需的累计经验：25·n·(n-1)，即 0、50、150、300、500…
func PetLevelThreshold(level int) int { return 25 * level * (level - 1) }

// PetLevel 由累计经验算出等级，以及当前等级的起点和下一级的门槛（满级时 next 为 0）。
func PetLevel(exp int) (level, floor, next int) {
	level = 1
	for level < PetMaxLevel && exp >= PetLevelThreshold(level+1) {
		level++
	}
	floor = PetLevelThreshold(level)
	if level < PetMaxLevel {
		next = PetLevelThreshold(level + 1)
	}
	return
}

// PetUnlocks 返回 level 级已解锁的配饰与眼镜。
func PetUnlocks(level int) (accessories, glasses []string) {
	accessories, glasses = []string{}, []string{}
	for _, a := range petAccessories {
		if level >= a.Level {
			accessories = append(accessories, a.ID)
		}
	}
	for _, g := range petGlasses {
		if level >= g.Level {
			glasses = append(glasses, g.ID)
		}
	}
	return
}

// PetView 是给前端的宠物信息。
type PetView struct {
	store.Pet
	Level       int      `json:"level"`
	LevelFloor  int      `json:"levelFloor"`
	LevelNext   int      `json:"levelNext"` // 0 = 已满级
	Days        int      `json:"days"`      // 陪伴天数（从 1 开始）
	Accessories []string `json:"accessories"`
	GlassesList []string `json:"glassesList"`
	WaterToday  int      `json:"waterToday"` // 今天喝了几杯（日期不是今天时为 0）
}

func (s *Service) today() string { return s.now().In(s.loc()).Format("2006-01-02") }

func (s *Service) petView(p store.Pet) PetView {
	v := PetView{Pet: p}
	v.Level, v.LevelFloor, v.LevelNext = PetLevel(p.Exp)
	v.Accessories, v.GlassesList = PetUnlocks(v.Level)
	loc := s.loc()
	y1, m1, d1 := time.UnixMilli(p.CreatedAt).In(loc).Date()
	y2, m2, d2 := s.now().In(loc).Date()
	v.Days = int(time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC).Sub(time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)).Hours()/24) + 1
	if p.WaterDate == s.today() {
		v.WaterToday = p.WaterCount
	}
	return v
}

// GetPet 返回宠物；还没有时按默认名字创建（开启宠物功能后第一次调用）。
func (s *Service) GetPet() (PetView, error) {
	p, ok, err := s.st.GetPet()
	if err != nil {
		return PetView{}, err
	}
	if !ok {
		if p, err = s.st.SavePet(store.Pet{Name: PetDefaultName}); err != nil {
			return PetView{}, err
		}
	}
	return s.petView(p), nil
}

// RenamePet 给狗狗改名（1–12 个字），同时标记为「已起名」（第一次开启时的起名框也调用它）。
func (s *Service) RenamePet(name string) (PetView, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 12 {
		return PetView{}, errors.New("名字需要 1–12 个字")
	}
	p, err := s.GetPet()
	if err != nil {
		return p, err
	}
	p.Name, p.Named = name, true
	saved, err := s.st.SavePet(p.Pet)
	if err != nil {
		return PetView{}, err
	}
	v := s.petView(saved)
	s.Emit("pet:changed", v)
	return v, nil
}

// PatPet 摸摸头（只计数，不加经验）。
func (s *Service) PatPet() (PetView, error) { return s.petProgress(0, 0, 1, 0) }

// GivePetSnack 喂零食（只计数，不加经验——经验只来自完成事项）。
func (s *Service) GivePetSnack() (PetView, error) { return s.petProgress(0, 0, 0, 1) }

func (s *Service) petProgress(exp, completed, pats, snacks int) (PetView, error) {
	if _, err := s.GetPet(); err != nil {
		return PetView{}, err
	}
	p, _, err := s.st.AddPetProgress(exp, completed, pats, snacks)
	if err != nil {
		return PetView{}, err
	}
	return s.petView(p), nil
}

// SetPetLook 换装：只能选已解锁的配饰 / 眼镜。
func (s *Service) SetPetLook(accessory, glasses string) (PetView, error) {
	p, err := s.GetPet()
	if err != nil {
		return p, err
	}
	if !slices.Contains(p.Accessories, accessory) || !slices.Contains(p.GlassesList, glasses) {
		return p, errors.New("还没有解锁这个装扮")
	}
	p.Accessory, p.Glasses = accessory, glasses
	saved, err := s.st.SavePet(p.Pet)
	if err != nil {
		return PetView{}, err
	}
	v := s.petView(saved)
	s.Emit("pet:changed", v)
	return v, nil
}

// PlayBall 一起玩了一次球（FR 二期：扔个球）。
func (s *Service) PlayBall() (PetView, error) {
	if _, err := s.GetPet(); err != nil {
		return PetView{}, err
	}
	p, _, err := s.st.AddPetPlay()
	if err != nil {
		return PetView{}, err
	}
	return s.petView(p), nil
}

// DrinkWater 喝水提醒点了「好了」：今天的杯数加一。
func (s *Service) DrinkWater() (PetView, error) {
	if _, err := s.GetPet(); err != nil {
		return PetView{}, err
	}
	p, _, err := s.st.AddPetWater(s.today())
	if err != nil {
		return PetView{}, err
	}
	return s.petView(p), nil
}

// ResetPet 重置成长进度（名字和装扮也恢复默认，下次显示时重新弹出起名框）。
func (s *Service) ResetPet() (PetView, error) {
	saved, err := s.st.SavePet(store.Pet{Name: PetDefaultName})
	if err != nil {
		return PetView{}, err
	}
	v := s.petView(saved)
	s.Emit("pet:changed", v)
	return v, nil
}

// PetEvent 是宠物相关的事件（前端据此播放动画）。
type PetEvent struct {
	Kind    string  `json:"kind"` // fed
	Exp     int     `json:"exp"`
	LevelUp bool    `json:"levelUp"`
	Pet     PetView `json:"pet"`
}

// petOnComplete 完成事项时调用：开启宠物时加经验并通知前端；返回实际加了多少（供撤销时扣回）。
func (s *Service) petOnComplete() int {
	if !s.GetSettings().PetEnabled {
		return 0
	}
	before, err := s.GetPet()
	if err != nil {
		return 0
	}
	p, ok, err := s.st.AddPetProgress(PetExpPerItem, 1, 0, 0)
	if err != nil || !ok {
		return 0
	}
	v := s.petView(p)
	s.Emit("pet:event", PetEvent{Kind: "fed", Exp: PetExpPerItem, LevelUp: v.Level > before.Level, Pet: v})
	return PetExpPerItem
}

// petUndoComplete 撤销完成时扣回经验。
func (s *Service) petUndoComplete(exp int) {
	if exp == 0 {
		return
	}
	if p, ok, err := s.st.AddPetProgress(-exp, -1, 0, 0); err == nil && ok {
		s.Emit("pet:changed", s.petView(p))
	}
}

// PetStatus 是宠物决定表情所需的事项信息（逾期数、今天的事项完成情况）。
type PetStatus struct {
	Pet        PetView `json:"pet"`
	Overdue    int     `json:"overdue"`
	TodayTotal int     `json:"todayTotal"` // 今天到期的事项（含已完成）
	TodayLeft  int     `json:"todayLeft"`
}

// PetStatus 返回宠物及其关心的事项统计。
func (s *Service) PetStatus() (PetStatus, error) {
	p, err := s.GetPet()
	if err != nil {
		return PetStatus{}, err
	}
	st := PetStatus{Pet: p, Overdue: s.st.CountOverdue()}
	all, err := s.st.ListItems(store.Filter{})
	if err != nil {
		return st, err
	}
	now, loc := s.now(), s.loc()
	y, m, d := now.In(loc).Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc).UnixMilli()
	end := time.Date(y, m, d+1, 0, 0, 0, 0, loc).UnixMilli()
	for _, it := range all {
		if it.DueAt == nil || *it.DueAt < start || *it.DueAt >= end || it.Status == store.StatusDeleted {
			continue
		}
		st.TodayTotal++
		if it.Status == store.StatusTodo {
			st.TodayLeft++
		}
	}
	return st, nil
}
