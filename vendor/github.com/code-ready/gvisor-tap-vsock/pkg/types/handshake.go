package types

type DeviceType string

const (
	TAP DeviceType = "tap"
	TUN DeviceType = "tun"
)

type HandshakeRequest struct {
	Hostname   string     `json:"hostname"`
	DeviceType DeviceType `json:"device_type"`
}

type HandshakeResponse struct {
	MTU     int    `json:"mtu"`
	Gateway string `json:"gateway"`
	VM      string `json:"vm"`
}

type ExposeRequest struct {
	Local  string `json:"local"`
	Remote string `json:"remote"`
}

type UnexposeRequest struct {
	Local string `json:"local"`
}
