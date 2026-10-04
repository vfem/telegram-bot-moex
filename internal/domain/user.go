package domain

import "time"

// BondGroup represents a named collection of bonds defined by a user (e.g. "OFZ", "HighYield").
type BondGroup struct {
	Name  string   `json:"name"`
	ISINs []string `json:"isins"`
}

// User represents a Telegram user with their settings, custom groups, and subscriptions.
type User struct {
	TelegramID          int64                `json:"telegram_id"`
	ChatID              int64                `json:"chat_id"`
	Groups              map[string]BondGroup `json:"groups"` // Group name -> BondGroup
	MonitoredISINs      []string             `json:"monitored_isins"`
	NotifyDailyPayments bool                 `json:"notify_daily_payments"`
	NotifyAnnouncements bool                 `json:"notify_announcements"`
	UpdatedAt           time.Time            `json:"updated_at"`
}

// NewUser creates a new initialized User.
func NewUser(telegramID, chatID int64) *User {
	return &User{
		TelegramID:     telegramID,
		ChatID:         chatID,
		Groups:         make(map[string]BondGroup),
		MonitoredISINs: make([]string, 0),
		UpdatedAt:      time.Now(),
	}
}

// AddBondToGroup adds an ISIN to a specified named group.
func (u *User) AddBondToGroup(groupName, isin string) {
	group, exists := u.Groups[groupName]
	if !exists {
		group = BondGroup{
			Name:  groupName,
			ISINs: make([]string, 0),
		}
	}
	for _, item := range group.ISINs {
		if item == isin {
			return
		}
	}
	group.ISINs = append(group.ISINs, isin)
	u.Groups[groupName] = group
	u.UpdatedAt = time.Now()
}

// AddMonitoredISIN adds an ISIN to general monitoring.
func (u *User) AddMonitoredISIN(isin string) {
	for _, item := range u.MonitoredISINs {
		if item == isin {
			return
		}
	}
	u.MonitoredISINs = append(u.MonitoredISINs, isin)
	u.UpdatedAt = time.Now()
}
