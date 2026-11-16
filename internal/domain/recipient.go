package domain

// Recipient is the minimal address-book projection for mail-client completion.
// Notes, biographies and other CRM fields are deliberately not included.
type Recipient struct {
	ContactID    string `json:"contact_id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	Organization string `json:"organization"`
	Preferred    bool   `json:"preferred"`
}
