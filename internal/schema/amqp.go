package schema

type AmqpMessage struct {
	ByteMsg    []byte
	Priority   uint8
	RoutingKey string
	Expiration int64
}
