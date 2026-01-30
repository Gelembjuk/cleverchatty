package shared

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// Email represents an email message
type Email struct {
	ID      string    `json:"id"`
	From    string    `json:"from"`
	Subject string    `json:"subject"`
	Body    string    `json:"body"`
	Read    bool      `json:"read"`
	SentAt  time.Time `json:"sent_at"`
}

// EmailManager manages all emails with thread-safe operations
type EmailManager struct {
	mu     sync.RWMutex
	emails []*Email
}

func NewEmailManager() *EmailManager {
	return &EmailManager{
		emails: make([]*Email, 0),
	}
}

func (em *EmailManager) AddEmail(email *Email) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.emails = append(em.emails, email)
}

func (em *EmailManager) GetEmails() []*Email {
	em.mu.RLock()
	defer em.mu.RUnlock()

	// Return a copy
	emails := make([]*Email, len(em.emails))
	copy(emails, em.emails)
	return emails
}

func (em *EmailManager) GetUnreadCount() int {
	em.mu.RLock()
	defer em.mu.RUnlock()

	count := 0
	for _, email := range em.emails {
		if !email.Read {
			count++
		}
	}
	return count
}

func (em *EmailManager) MarkAsRead(emailID string) bool {
	em.mu.Lock()
	defer em.mu.Unlock()

	for _, email := range em.emails {
		if email.ID == emailID {
			email.Read = true
			return true
		}
	}
	return false
}

// Random email data for generation
var (
	firstNames = []string{
		"Alice", "Bob", "Charlie", "Diana", "Edward", "Fiona", "George",
		"Hannah", "Isaac", "Julia", "Kevin", "Laura", "Michael", "Nancy",
	}

	lastNames = []string{
		"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller",
		"Davis", "Rodriguez", "Martinez", "Hernandez", "Lopez", "Wilson",
	}

	domains = []string{
		"example.com", "email.com", "mail.com", "company.com",
		"business.org", "tech.io", "startup.net",
	}

	subjects = []string{
		"Meeting reminder for tomorrow",
		"Project update and next steps",
		"Question about the proposal",
		"Weekly report submission",
		"Important: System maintenance scheduled",
		"Invoice #%d for your review",
		"Thank you for your feedback",
		"Follow-up on our conversation",
		"New feature announcement",
		"Action required: Please review",
		"Team lunch next week",
		"Conference invitation",
		"Budget approval needed",
		"Client feedback received",
		"Performance review scheduled",
	}

	bodyTemplates = []string{
		"Hi there,\n\nJust wanted to follow up on our previous discussion. Please let me know your thoughts.\n\nBest regards,\n%s",
		"Hello,\n\nI hope this email finds you well. I wanted to share some updates about the project.\n\nThanks,\n%s",
		"Dear colleague,\n\nPlease find attached the document we discussed. Let me know if you have any questions.\n\nRegards,\n%s",
		"Hi,\n\nQuick reminder about the meeting scheduled for tomorrow at 2 PM.\n\nSee you then!\n%s",
		"Hello,\n\nI've reviewed your proposal and have some feedback to share. Can we schedule a call?\n\nBest,\n%s",
		"Hey,\n\nGreat work on the presentation! Looking forward to the next steps.\n\nCheers,\n%s",
		"Hi there,\n\nJust checking in to see if you need any help with the current task.\n\nThanks,\n%s",
	}
)

// GenerateRandomEmail creates a random email message
func GenerateRandomEmail() *Email {
	firstName := firstNames[rand.Intn(len(firstNames))]
	lastName := lastNames[rand.Intn(len(lastNames))]
	domain := domains[rand.Intn(len(domains))]

	from := fmt.Sprintf("%s.%s@%s",
		firstName, lastName, domain)

	// Select a random subject
	subject := subjects[rand.Intn(len(subjects))]
	// If it's the invoice subject (contains %d), format it with a random number
	if strings.Contains(subject, "%d") {
		subject = fmt.Sprintf(subject, rand.Intn(9999)+1)
	}

	bodyTemplate := bodyTemplates[rand.Intn(len(bodyTemplates))]
	body := fmt.Sprintf(bodyTemplate, firstName)

	return &Email{
		ID:      fmt.Sprintf("email_%d", time.Now().UnixNano()),
		From:    from,
		Subject: subject,
		Body:    body,
		Read:    false,
		SentAt:  time.Now(),
	}
}

// GetRandomDelay returns a random duration between 5 and 15 seconds
func GetRandomDelay() time.Duration {
	seconds := 5 + rand.Intn(11) // 5 to 15 seconds
	return time.Duration(seconds) * time.Second
}
