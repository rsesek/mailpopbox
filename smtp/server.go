package smtp

type Server interface {
	OnEHLO() error
	OnMessageDelivered() error
}
