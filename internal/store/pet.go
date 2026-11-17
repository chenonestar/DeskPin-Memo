package store

import "database/sql"

// Pet 是桌面宠物（只有一只）。
type Pet struct {
	Name       string `json:"name"`
	Named      bool   `json:"named"` // 用户确认过名字（第一次开启时弹出起名框）
	Exp        int    `json:"exp"`
	Accessory  string `json:"accessory"`
	Glasses    string `json:"glasses"`
	Completed  int    `json:"completed"` // 陪你完成的事项数
	Pats       int    `json:"pats"`
	Snacks     int    `json:"snacks"`
	Plays      int    `json:"plays"`      // 一起玩球的次数
	WaterDate  string `json:"waterDate"`  // WaterCount 所属日期（本地日期 YYYY-MM-DD）
	WaterCount int    `json:"waterCount"` // 当天喝水杯数
	CreatedAt  int64  `json:"createdAt"`
}

const petCols = `name,named,exp,accessory,glasses,completed,pats,snacks,plays,water_date,water_count,created_at`

// GetPet 读取宠物；还没有养时 ok 为 false。
func (s *Store) GetPet() (Pet, bool, error) {
	return scanPet(s.db.QueryRow(`SELECT ` + petCols + ` FROM pet WHERE id=1`))
}

func scanPet(r *sql.Row) (Pet, bool, error) {
	var p Pet
	var named int
	err := r.Scan(&p.Name, &named, &p.Exp, &p.Accessory, &p.Glasses, &p.Completed, &p.Pats, &p.Snacks, &p.Plays, &p.WaterDate, &p.WaterCount, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return p, false, nil
	}
	p.Named = named == 1
	return p, err == nil, err
}

// savePetSQL 新建或整体更新宠物（导入时也用）。
const savePetSQL = `INSERT INTO pet(id,` + petCols + `) VALUES(1,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(id) DO UPDATE SET name=excluded.name, named=excluded.named, exp=excluded.exp, accessory=excluded.accessory,
	glasses=excluded.glasses, completed=excluded.completed, pats=excluded.pats, snacks=excluded.snacks, plays=excluded.plays,
	water_date=excluded.water_date, water_count=excluded.water_count, created_at=excluded.created_at`

func petArgs(p Pet) []any {
	return []any{p.Name, b2i(p.Named), p.Exp, p.Accessory, p.Glasses, p.Completed, p.Pats, p.Snacks, p.Plays, p.WaterDate, p.WaterCount, p.CreatedAt}
}

// SavePet 新建或整体更新宠物。CreatedAt 为 0 时取当前时间。
func (s *Store) SavePet(p Pet) (Pet, error) {
	if p.CreatedAt == 0 {
		p.CreatedAt = s.nowMs()
	}
	if p.Glasses == "" {
		p.Glasses = "round"
	}
	_, err := s.db.Exec(savePetSQL, petArgs(p)...)
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

// AddPetPlay 一起玩球次数加一。
func (s *Store) AddPetPlay() (Pet, bool, error) {
	if _, err := s.db.Exec(`UPDATE pet SET plays=plays+1 WHERE id=1`); err != nil {
		return Pet{}, false, err
	}
	return s.GetPet()
}

// AddPetWater 记录喝了一杯水：日期变了从 1 重新计。
func (s *Store) AddPetWater(today string) (Pet, bool, error) {
	if _, err := s.db.Exec(`UPDATE pet SET water_count=CASE WHEN water_date=? THEN water_count+1 ELSE 1 END, water_date=? WHERE id=1`, today, today); err != nil {
		return Pet{}, false, err
	}
	return s.GetPet()
}
