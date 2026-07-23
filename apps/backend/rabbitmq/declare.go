package rabbitmq

// DeclareTopology sets up exchanges, queues, and bindings using the given
// client. The client is required so that, when a queue redeclare triggers a
// PRECONDITION_FAILED (406) that closes the channel, a fresh channel can be
// opened to recover.
func DeclareTopology(c *Client) error {
	return declareTopology(c)
}
