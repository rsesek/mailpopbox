package smtp

type Server interface {
	Name() string
	OnEHLO() error
	OnMessageDelivered() error
}
