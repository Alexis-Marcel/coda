package server

import (
	"net/http"

	"github.com/alexis-marcel/gamekit/auth"
)

// Comptes : facultatifs, selon la méthode choisie par l'hébergeur (voir
// gamekit/auth). La place d'un joueur est tenue par son jeton de place.

type User = auth.User

// user renvoie le compte de la requête, nil en invité.
func (s *Server) user(r *http.Request) *User { return s.auth.User(r) }
