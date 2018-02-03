package config

// A Guesser accepts a basename and gives the most possibly suitable category name.
type Guesser interface {
	Guess(string) string
}

// NoGuessing does not perform any form of guessing, simply returning the default
type noGuessing struct{}

func (r noGuessing) Guess(_ string) string {
	return Config.DefaultCategory
}

var NoGuessing noGuessing
