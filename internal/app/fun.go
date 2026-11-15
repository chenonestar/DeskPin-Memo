package app

import (
	"deskpinmemo/internal/service"
	"deskpinmemo/internal/store"
)

// ---------------------------------------------------------------- 摸鱼：RSS 新闻（FR-701~707）

func (a *App) FeedPresets() ([]service.PresetView, error)        { return a.svc.FeedPresets() }
func (a *App) SetupNews(urls []string) (service.Settings, error) { return a.svc.SetupNews(urls) }
func (a *App) ListFeeds() ([]store.Feed, error)                  { return a.svc.ListFeeds() }
func (a *App) AddFeed(url string) (store.Feed, error)            { return a.svc.AddFeed(url) }
func (a *App) UpdateFeed(id string, title *string, enabled *bool) (store.Feed, error) {
	return a.svc.UpdateFeed(id, title, enabled)
}
func (a *App) DeleteFeed(id string) error           { return a.svc.DeleteFeed(id) }
func (a *App) RefreshNews() (int, error)            { return a.svc.RefreshNews() }
func (a *App) TestFeed(url string) service.FeedTest { return a.svc.TestFeed(url) }
func (a *App) NewsUnread() int                      { return a.svc.NewsUnread() }
func (a *App) MarkAllNewsRead(feedID string) error  { return a.svc.MarkAllNewsRead(feedID) }
func (a *App) NewsItems(feedID string, limit int) ([]store.FeedItem, error) {
	return a.svc.NewsItems(feedID, limit)
}

// OpenNewsItem 用默认浏览器打开新闻原文（只允许 http / https），并标为已读。
func (a *App) OpenNewsItem(id string) error {
	link, err := a.svc.OpenNewsItem(id)
	if err != nil {
		return err
	}
	return a.shell.OpenPath(link)
}

// ---------------------------------------------------------------- 摸鱼：戴眼镜的狗狗（FR-801~810）

func (a *App) GetPet() (service.PetView, error)               { return a.svc.GetPet() }
func (a *App) PetStatus() (service.PetStatus, error)          { return a.svc.PetStatus() }
func (a *App) RenamePet(name string) (service.PetView, error) { return a.svc.RenamePet(name) }
func (a *App) PatPet() (service.PetView, error)               { return a.svc.PatPet() }
func (a *App) GivePetSnack() (service.PetView, error)         { return a.svc.GivePetSnack() }
func (a *App) ResetPet() (service.PetView, error)             { return a.svc.ResetPet() }
func (a *App) SetPetLook(acc, glasses string) (service.PetView, error) {
	return a.svc.SetPetLook(acc, glasses)
}

// IdleSeconds 返回整个系统的空闲秒数（狗狗打盹用）。
func (a *App) IdleSeconds() int { return a.shell.IdleSeconds() }

// ---------------------------------------------------------------- 摸鱼：正经模式（老板键）

// BossMode 返回是否处于正经模式。
func (a *App) BossMode() bool { return a.boss }

// ToggleBossMode 切换正经模式（全局快捷键 / 托盘菜单）。
func (a *App) ToggleBossMode() (bool, error) { return a.SetBossMode(!a.boss) }

// SetBossMode 进入 / 退出正经模式。进入时收起新闻条、宠物、倒计时；设置了「切到的分组」时一并切过去，
// 退出时切回原来的分组。只是界面状态，不保存（重启后恢复正常）。
func (a *App) SetBossMode(on bool) (bool, error) {
	if on == a.boss {
		return a.boss, nil
	}
	if on {
		a.bossPrev = ""
		if g := a.svc.GetSettings().BossGroupID; g != "" && g != a.groupID && a.groupExists(g) {
			a.bossPrev = a.groupID
			if err := a.switchGroup(g); err != nil {
				return a.boss, err
			}
		}
	} else if a.bossPrev != "" {
		prev := a.bossPrev
		a.bossPrev = ""
		if a.groupExists(prev) {
			if err := a.switchGroup(prev); err != nil {
				return a.boss, err
			}
		}
	}
	a.boss = on
	a.Emit("boss:changed", on)
	return on, nil
}

func (a *App) groupExists(id string) bool {
	gs, err := a.svc.Groups()
	if err != nil {
		return false
	}
	for _, g := range gs {
		if g.ID == id {
			return true
		}
	}
	return false
}

// switchGroup 由后端发起的分组切换：切换后通知前端跟着显示该分组。
func (a *App) switchGroup(id string) error {
	ws, err := a.ShowGroup(id)
	if err != nil {
		return err
	}
	a.Emit("window:state", ws)
	a.Emit("group:show", id)
	return nil
}
