package ledger

import (
	"database/sql"
	"errors"
	"strings"
)

// Category is one node of the two-level tree. A transaction may point at a
// parent ("food") or a leaf ("food.groceries"); reports roll leaves up.
type Category struct {
	ID        string `json:"id"`
	Parent    string `json:"parent,omitempty"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // income | expense | transfer
	Essential bool   `json:"essential"`
	Color     string `json:"color,omitempty"`
	Sort      int    `json:"sort"`
	Archived  bool   `json:"archived"`
}

// Top returns the top-level category id of a path ("food.groceries" → "food").
func Top(id string) string {
	if i := strings.IndexByte(id, '.'); i > 0 {
		return id[:i]
	}
	return id
}

type seedCat struct {
	id, name, kind string
	essential      bool
	color          string
	children       [][2]string // id suffix, name
}

// Seed is the default tree. Expense colors follow a fixed categorical order
// so every chart uses the same hue for the same category.
var seed = []seedCat{
	{"salary", "Salary", "income", false, "", nil},
	{"side_income", "Side income & rent", "income", false, "", nil},
	{"benefits", "Benefits", "income", false, "", nil},
	{"refunds", "Refunds & reimbursements", "income", false, "", nil},
	{"investment_income", "Investment income", "income", false, "", nil},
	{"other_income", "Other income", "income", false, "", nil},

	{"housing", "Housing", "expense", true, "#4e79a7", [][2]string{
		{"mortgage_interest", "Mortgage interest"}, {"mortgage", "Mortgage payment (combined)"},
		{"rent", "Rent"}, {"maintenance", "Repairs & DIY"}, {"furnishing", "Furniture & home"},
		{"insurance", "Home insurance"}, {"services", "Home services"},
	}},
	{"utilities", "Utilities", "expense", true, "#59a14f", [][2]string{
		{"electricity", "Electricity"}, {"heating", "Heating & gas"}, {"water", "Water"},
		{"telecom", "Phone & internet"}, {"building", "Building admin"}, {"security", "Security"},
	}},
	{"food", "Food", "expense", true, "#f28e2b", [][2]string{
		{"groceries", "Groceries"}, {"restaurants", "Restaurants"}, {"fast_food", "Fast food"},
		{"delivery", "Delivery"}, {"coffee", "Coffee"}, {"work_lunch", "Work lunch"},
	}},
	{"transport", "Transport", "expense", true, "#76b7b2", [][2]string{
		{"fuel", "Fuel"}, {"parking", "Parking"}, {"car", "Car service & wash"},
		{"taxi", "Taxi & public transport"}, {"car_fees", "Car insurance & tax"},
	}},
	{"kids", "Kids", "expense", true, "#edc948", [][2]string{
		{"alimony", "Alimony"}, {"education", "School & activities"}, {"general", "Kids general"},
	}},
	{"health", "Health", "expense", false, "#e15759", [][2]string{
		{"pharmacy", "Pharmacy"}, {"medical", "Doctors & dental"}, {"therapy", "Therapy"},
		{"fitness", "Sport & gym"}, {"care", "Beauty & haircut"},
	}},
	{"shopping", "Shopping", "expense", false, "#b07aa1", [][2]string{
		{"online", "Marketplaces"}, {"electronics", "Electronics"}, {"clothing", "Clothing"}, {"other", "Other shopping"},
	}},
	{"leisure", "Leisure", "expense", false, "#ff9da7", [][2]string{
		{"going_out", "Bars & nightlife"}, {"events", "Cinema & events"}, {"hobbies", "Hobbies & games"}, {"lottery", "Lottery"},
	}},
	{"travel", "Travel", "expense", false, "#9c755f", [][2]string{
		{"flights", "Flights"}, {"lodging", "Hotels & rentals"}, {"trip", "On the trip"},
	}},
	{"dating", "Dating", "expense", false, "#d37295", nil},
	{"subscriptions", "Subscriptions", "expense", false, "#8cd17d", [][2]string{
		{"software", "Software & AI"}, {"media", "Media & streaming"}, {"creators", "Creators & donations"},
	}},
	{"gifts", "Gifts", "expense", false, "#f1ce63", nil},
	{"finance", "Finance & fees", "expense", true, "#79706e", [][2]string{
		{"bank_fees", "Bank fees"}, {"taxes", "Taxes"}, {"insurance", "Life insurance"},
		{"legal", "Legal & notary"}, {"divorce", "Divorce"}, {"fines", "Fines"},
	}},
	{"other", "Other", "expense", false, "#bab0ac", [][2]string{
		{"family", "Family support"}, {"cash", "Untracked cash"}, {"education", "Self-education"},
	}},

	{"transfer", "Transfers", "transfer", false, "", [][2]string{
		{"internal", "Between own accounts"}, {"invest", "Investing"}, {"pension", "Pension"},
		{"debt", "Debt principal"}, {"asset", "Asset purchase"},
	}},
}

// essentialLeaves overrides the parent's essential flag for specific leaves.
var essentialLeaves = map[string]bool{
	"food.groceries": true, "food.restaurants": false, "food.fast_food": false,
	"food.delivery": false, "food.coffee": false, "food.work_lunch": false,
	"housing.furnishing": false, "housing.maintenance": false,
	"health.pharmacy": true, "health.medical": true, "health.therapy": true,
}

// SeedCategories inserts the default tree, leaving existing rows alone.
func SeedCategories(db execer) error {
	sort := 0
	for _, c := range seed {
		sort += 10
		if _, err := db.Exec(`INSERT OR IGNORE INTO categories(id,parent,name,kind,essential,color,sort) VALUES(?,NULL,?,?,?,?,?)`,
			c.id, c.name, c.kind, c.essential, c.color, sort); err != nil {
			return err
		}
		for i, ch := range c.children {
			id := c.id + "." + ch[0]
			ess := c.essential
			if v, ok := essentialLeaves[id]; ok {
				ess = v
			}
			if _, err := db.Exec(`INSERT OR IGNORE INTO categories(id,parent,name,kind,essential,color,sort) VALUES(?,?,?,?,?,?,?)`,
				id, c.id, ch[1], c.kind, ess, "", sort+i+1); err != nil {
				return err
			}
		}
	}
	return nil
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// ListCategories returns the whole tree in display order.
func ListCategories(db querier) ([]Category, error) {
	rows, err := db.Query(`SELECT id, COALESCE(parent,''), name, kind, essential, color, sort, archived FROM categories ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Parent, &c.Name, &c.Kind, &c.Essential, &c.Color, &c.Sort, &c.Archived); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CategoryMap indexes categories by id.
func CategoryMap(db querier) (map[string]Category, error) {
	list, err := ListCategories(db)
	if err != nil {
		return nil, err
	}
	m := make(map[string]Category, len(list))
	for _, c := range list {
		m[c.ID] = c
	}
	return m, nil
}

// SaveCategory creates or updates a category. New ids derive from the name
// under the parent ("food" + "Bakery" → "food.bakery").
func SaveCategory(d *sql.DB, c *Category) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.New("name is required")
	}
	var exists int
	if c.ID != "" {
		d.QueryRow(`SELECT COUNT(*) FROM categories WHERE id=?`, c.ID).Scan(&exists)
	}
	if exists > 0 {
		_, err := d.Exec(`UPDATE categories SET name=?, essential=?, color=?, sort=?, archived=? WHERE id=?`, c.Name, c.Essential, c.Color, c.Sort, c.Archived, c.ID)
		return err
	}
	slug := strings.ReplaceAll(SlugID(c.Name), "-", "_")
	if c.Parent != "" {
		var kind string
		if err := d.QueryRow(`SELECT kind FROM categories WHERE id=? AND parent IS NULL`, c.Parent).Scan(&kind); err != nil {
			return errors.New("parent must be a top-level category")
		}
		c.Kind = kind
		c.ID = c.Parent + "." + slug
	} else {
		if c.Kind != "income" && c.Kind != "expense" && c.Kind != "transfer" {
			return errors.New("kind must be income, expense or transfer")
		}
		c.ID = slug
	}
	var parent any
	if c.Parent != "" {
		parent = c.Parent
	}
	if c.Sort == 0 {
		d.QueryRow(`SELECT COALESCE(MAX(sort),0)+1 FROM categories`).Scan(&c.Sort)
	}
	_, err := d.Exec(`INSERT INTO categories(id,parent,name,kind,essential,color,sort) VALUES(?,?,?,?,?,?,?)`, c.ID, parent, c.Name, c.Kind, c.Essential, c.Color, c.Sort)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return errors.New("a category with this name already exists")
	}
	return err
}
