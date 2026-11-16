package campaign

// ChannelProgress is how far one channel has come. Pending includes the rows
// being sent right now and the ones waiting for a retry.
type ChannelProgress struct {
	Total   int `json:"total"`
	Pending int `json:"pending"`
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

// DeliveryView is the sending progress of an approved campaign.
type DeliveryView struct {
	SMS   ChannelProgress `json:"sms"`
	Email ChannelProgress `json:"email"`
	// Done is true once nothing is pending in either channel.
	Done bool `json:"done"`
}
