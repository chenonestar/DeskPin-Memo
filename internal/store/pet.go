package store

import "database/sql"

// Pet 是桌面宠物（只有一只）。
type Pet struct {
	Name      string `json:"name"`
	Exp       int    `json:"exp"`
	Accessory string `json:"accessory"`
	Glasses   string `json:"glasses"`
	Completed int    `json:"completed"` // 陪你完成的事项数
	Pats      int    `json:"pats"`
	Snacks    int    `json:"snacks"`
	CreatedAt int64  `json:"createdAt"`
}

// GetPet 读取宠物；还没有养时 ok 为 false。
func (s *Store) GetPet() (Pet, bool, error) {
	var p Pet
	err := s.db.QueryRow(`SELECT name,exp,accessory,glasses,completed,pats,snacks,created_at FROM pet WHERE id=1`).
		Scan(&p.Name, &p.Exp, &p.Accessory, &p.Glasses, &p.Completed, &p.Pats, &p.Snacks, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return p, false, nil
	}
	return p, err == nil, err
}

// SavePet 新建或整体更新宠物。CreatedAt 为 0 时取当前时间。
func (s *Store) SavePet(p Pet) (Pet, error) {
	if p.CreatedAt == 0 {
		p.CreatedAt = s.nowMs()
	}
	if p.Glasses == "" {
		p.Glasses = "round"
	}
	_, err := s.db.Exec(`INSERT INTO pet(id,name,exp,accessory,glasses,completed,pats,snacks,created_at) VALUES(1,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, exp=excluded.exp, accessory=excluded.accessory, glasses=excluded.glasses,
		completed=excluded.completed, pats=excluded.pats, snacks=excluded.snacks, created_at=excluded.created_at`,
		p.Name, p.Exp, p.Accessory, p.Glasses, p.Completed, p.Pats, p.Snacks, p.CreatedAt)
	return p, err
}

// AddPetProgress 原子地增加经验 / 完成数 / 摸头数 / 零食数（可为负，结果不小于 0）。没有宠物时什么也不做。
func (s *Store) AddPetProgress(exp, completed, pats, snacks int) (Pet, bool, error) {
	_, err := s.db.Exec(`UPDATE pet SET exp=MAX(0,exp+?), completed=MAX(0,completed+?), pats=MAX(0,pats+?), snacks=MAX(0,snacks+?) WHERE id=1`,
		exp, completed, pats, snacks)
	if err != nil {
		return Pet{}, false, err
	}
	return s.GetPet()
}
