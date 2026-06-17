package storage

import "time"

const (
	DeviceStatusPending  = "pending"
	DeviceStatusApproved = "approved"
	DeviceStatusRevoked  = "revoked"
)

const (
	DeviceKindDesktop = "desktop"
	DeviceKindMobile  = "mobile"
	DeviceKindBrowser = "browser"
	DeviceKindCLI     = "cli"
)

const (
	PairingStatusPending  = "pending"
	PairingStatusApproved = "approved"
	PairingStatusRejected = "rejected"
	PairingStatusExpired  = "expired"
)

type Device struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	ApprovedAt time.Time `json:"approved_at,omitempty"`
	RevokedAt  time.Time `json:"revoked_at,omitempty"`
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`
	LastSeenIP string    `json:"last_seen_ip,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	AppVersion string    `json:"app_version,omitempty"`
}

type CreateDevice struct {
	ID         string
	Name       string
	Kind       string
	Status     string
	TokenHash  string
	CreatedAt  time.Time
	ApprovedAt time.Time
	LastSeenAt time.Time
	LastSeenIP string
	UserAgent  string
	AppVersion string
}

type DevicePairing struct {
	ID         string    `json:"id"`
	DeviceID   string    `json:"device_id"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	ApprovedAt time.Time `json:"approved_at,omitempty"`
	RejectedAt time.Time `json:"rejected_at,omitempty"`
	Device     Device    `json:"device"`
}

type CreateDevicePairing struct {
	ID         string
	DeviceID   string
	SecretHash string
	Status     string
	CreatedAt  time.Time
	ExpiresAt  time.Time
}
