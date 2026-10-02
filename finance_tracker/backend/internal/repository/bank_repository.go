package repository

import (
	"errors"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// StagedFilter narrows the staging list. Zero values mean "no filter".
type StagedFilter struct {
	State    string
	Verdict  string
	LinkID   uint
	Page     int
	PageSize int
}

// BankRepository stores the open-banking connection state and the staged rows
// awaiting the user's verdict.
type BankRepository interface {
	// Settings
	GetSettings() (*domain.BankSettings, error)
	SaveSettings(s *domain.BankSettings) error

	// Connections
	ListConnections() ([]domain.BankConnection, error)
	GetConnection(id uint) (*domain.BankConnection, error)
	GetConnectionByState(state string) (*domain.BankConnection, error)
	SaveConnection(c *domain.BankConnection) error
	DeleteConnection(id uint) error

	// Account links
	ListLinks(connectionID uint) ([]domain.BankAccountLink, error)
	ListAllLinks() ([]domain.BankAccountLink, error)
	GetLink(id uint) (*domain.BankAccountLink, error)
	GetLinkByHash(connectionID uint, hash string) (*domain.BankAccountLink, error)
	SaveLink(l *domain.BankAccountLink) error

	// Staged transactions
	GetStagedByExternalID(externalID string) (*domain.BankStagedTx, error)
	SaveStaged(t *domain.BankStagedTx) error
	ListStaged(f StagedFilter) ([]domain.BankStagedTx, int64, error)
	GetStagedByIDs(ids []uint) ([]domain.BankStagedTx, error)
	CountStagedByVerdict() (map[string]int, error)
	CountPendingByLink() (map[uint]int, error)
	// RestoreByImportedTxIDs puts rows whose committed transaction was just
	// deleted back on the review list. This is what makes undo return you to
	// where you were rather than silently dropping the row.
	RestoreByImportedTxIDs(txIDs []uint) error
}

type bankRepository struct {
	db *gorm.DB
}

func NewBankRepository(db *gorm.DB) BankRepository {
	return &bankRepository{db: db}
}

// --- settings ---

func (r *bankRepository) GetSettings() (*domain.BankSettings, error) {
	var s domain.BankSettings
	if err := r.db.FirstOrCreate(&s, domain.BankSettings{ID: 1}).Error; err != nil {
		return nil, err
	}
	if s.Environment == "" {
		s.Environment = domain.BankEnvSandbox
	}
	return &s, nil
}

func (r *bankRepository) SaveSettings(s *domain.BankSettings) error {
	s.ID = 1
	// Silence SQL logging for this one write: the row holds the plaintext RSA
	// private key, and GORM's Warn/Info logger interpolates bound values into
	// any slow-or-errored statement it prints. A bank credential must never
	// reach the log sink.
	return r.db.Session(&gorm.Session{Logger: r.db.Logger.LogMode(logger.Silent)}).Save(s).Error
}

// --- connections ---

func (r *bankRepository) ListConnections() ([]domain.BankConnection, error) {
	var out []domain.BankConnection
	err := r.db.Order("created_at DESC").Find(&out).Error
	return out, err
}

func (r *bankRepository) GetConnection(id uint) (*domain.BankConnection, error) {
	var c domain.BankConnection
	if err := r.db.First(&c, id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// GetConnectionByState resolves the single-use CSRF nonce minted when the auth
// URL was handed out. An expired nonce is treated as no match: a code replayed
// an hour later must not land on a live connection.
func (r *bankRepository) GetConnectionByState(state string) (*domain.BankConnection, error) {
	if state == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var c domain.BankConnection
	if err := r.db.Where("auth_state = ?", state).First(&c).Error; err != nil {
		return nil, err
	}
	if time.Since(c.AuthStateAt) > domain.AuthStateTTL {
		return nil, gorm.ErrRecordNotFound
	}
	return &c, nil
}

func (r *bankRepository) SaveConnection(c *domain.BankConnection) error {
	// The session id reads every transaction on the account — same logging
	// reasoning as the private key above.
	return r.db.Session(&gorm.Session{Logger: r.db.Logger.LogMode(logger.Silent)}).Save(c).Error
}

// DeleteConnection removes the connection, its account links and every staged
// row underneath it. Staged rows go too: without their link they can no longer
// be committed, and leaving them would show orphans on the review list.
func (r *bankRepository) DeleteConnection(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var linkIDs []uint
		if err := tx.Model(&domain.BankAccountLink{}).
			Where("connection_id = ?", id).Pluck("id", &linkIDs).Error; err != nil {
			return err
		}
		if len(linkIDs) > 0 {
			if err := tx.Where("link_id IN ?", linkIDs).Delete(&domain.BankStagedTx{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("connection_id = ?", id).Delete(&domain.BankAccountLink{}).Error; err != nil {
			return err
		}
		return tx.Delete(&domain.BankConnection{}, id).Error
	})
}

// --- account links ---

func (r *bankRepository) ListLinks(connectionID uint) ([]domain.BankAccountLink, error) {
	var out []domain.BankAccountLink
	err := r.db.Where("connection_id = ?", connectionID).Order("id").Find(&out).Error
	return out, err
}

func (r *bankRepository) ListAllLinks() ([]domain.BankAccountLink, error) {
	var out []domain.BankAccountLink
	err := r.db.Order("id").Find(&out).Error
	return out, err
}

func (r *bankRepository) GetLink(id uint) (*domain.BankAccountLink, error) {
	var l domain.BankAccountLink
	if err := r.db.First(&l, id).Error; err != nil {
		return nil, err
	}
	return &l, nil
}

// GetLinkByHash matches on identification_hash, not uid: the uid rotates on
// every re-authorisation, so matching on it would silently detach the account
// mapping each time consent is renewed.
func (r *bankRepository) GetLinkByHash(connectionID uint, hash string) (*domain.BankAccountLink, error) {
	if hash == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var l domain.BankAccountLink
	err := r.db.Where("connection_id = ? AND identification_hash = ?", connectionID, hash).First(&l).Error
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *bankRepository) SaveLink(l *domain.BankAccountLink) error {
	return r.db.Save(l).Error
}

// --- staged transactions ---

func (r *bankRepository) GetStagedByExternalID(externalID string) (*domain.BankStagedTx, error) {
	var t domain.BankStagedTx
	if err := r.db.Where("external_id = ?", externalID).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *bankRepository) SaveStaged(t *domain.BankStagedTx) error {
	return r.db.Save(t).Error
}

func (r *bankRepository) ListStaged(f StagedFilter) ([]domain.BankStagedTx, int64, error) {
	q := r.db.Model(&domain.BankStagedTx{})
	if f.State != "" {
		q = q.Where("state = ?", f.State)
	}
	if f.Verdict != "" {
		q = q.Where("verdict = ?", f.Verdict)
	}
	if f.LinkID != 0 {
		q = q.Where("link_id = ?", f.LinkID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	// Newest first: the rows most likely to be acted on are the recent ones.
	q = q.Order("date DESC, id DESC")
	if f.PageSize > 0 {
		q = q.Limit(f.PageSize).Offset((max(f.Page, 1) - 1) * f.PageSize)
	}
	var out []domain.BankStagedTx
	if err := q.Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *bankRepository) GetStagedByIDs(ids []uint) ([]domain.BankStagedTx, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []domain.BankStagedTx
	err := r.db.Where("id IN ?", ids).Order("date, id").Find(&out).Error
	return out, err
}

func (r *bankRepository) CountStagedByVerdict() (map[string]int, error) {
	type row struct {
		Verdict string
		N       int
	}
	var rows []row
	err := r.db.Model(&domain.BankStagedTx{}).
		Select("verdict, COUNT(*) AS n").
		Where("state = ?", domain.StagedStateStaged).
		Group("verdict").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, x := range rows {
		out[x.Verdict] = x.N
	}
	return out, nil
}

// CountPendingByLink powers the per-account badge.
//
// It counts every row still awaiting review, not just the genuinely new ones.
// A likely duplicate is not noise to be ignored — it sits in the queue until
// somebody looks at it and dismisses it, and a badge that leaves it out lets
// the queue grow while claiming it is empty. It also has to agree with the
// count on the transactions page, which labels the list the user actually
// works through: two numbers disagreeing on one screen is worse than either
// definition.
func (r *bankRepository) CountPendingByLink() (map[uint]int, error) {
	type row struct {
		LinkID uint
		N      int
	}
	var rows []row
	err := r.db.Model(&domain.BankStagedTx{}).
		Select("link_id, COUNT(*) AS n").
		Where("state = ?", domain.StagedStateStaged).
		Group("link_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uint]int, len(rows))
	for _, x := range rows {
		out[x.LinkID] = x.N
	}
	return out, nil
}

func (r *bankRepository) RestoreByImportedTxIDs(txIDs []uint) error {
	if len(txIDs) == 0 {
		return nil
	}
	err := r.db.Model(&domain.BankStagedTx{}).
		Where("imported_tx_id IN ? AND state = ?", txIDs, domain.StagedStateImported).
		Updates(map[string]any{
			"state":          domain.StagedStateStaged,
			"imported_tx_id": nil,
			// The row is new again by definition — the transaction it
			// matched has just been deleted.
			"verdict":       domain.VerdictNew,
			"verdict_note":  "",
			"matched_tx_id": nil,
		}).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}
