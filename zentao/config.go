package zentao

import (
	zentaoapi "github.com/myklst/terraform-provider-st-zentao/zentaoAPI"
)

type Config struct {
	URL      string
	Account  string
	Password string
}

func (c *Config) Client() (*zentaoapi.Client, error) {
	return zentaoapi.NewClient(c.URL, c.Account, c.Password)
}
