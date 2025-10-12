package internal

import "github.com/go-routeros/routeros/v3"

type RouterOS struct {
	client *routeros.Client
}

func NewRouterOS(client *routeros.Client) *RouterOS {
	return &RouterOS{client: client}
}

func (r *RouterOS) Cmd(cmd []string) (result []map[string]string, err error) {
	res, err := r.client.Run(cmd...)
	if err != nil {
		return result, err
	}
	// Iterujemy po odpowiedziach i dodajemy każdą mapę do slice
	for _, re := range res.Re {
		result = append(result, re.Map)
	}

	return result, nil
}
