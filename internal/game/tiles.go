// Package game est le moteur de règles de Coda : tuiles, pioche, annonces,
// sans I/O ni concurrence ; l'heure et le hasard sont passés en paramètre.
package game

import (
	"math/rand/v2"
	"slices"
)

// Color est la couleur d'une tuile, visible de tous.
type Color int

const (
	Black Color = 0
	White Color = 1
)

// MaxNum est la plus grande valeur d'une tuile : 0 à 11 dans chaque couleur.
const MaxNum = 11

// Tile est une tuile. ID est un repère tiré au hasard en début de partie : il
// désigne la tuile sans rien dire de sa valeur.
type Tile struct {
	ID       int   `json:"id"`
	Color    Color `json:"color"`
	Num      int   `json:"num"`
	Revealed bool  `json:"revealed,omitempty"`
}

// Less : ordre de la rangée, par valeur, la noire avant la blanche à valeur égale.
func Less(a, b Tile) bool {
	if a.Num != b.Num {
		return a.Num < b.Num
	}
	return a.Color < b.Color
}

func compare(a, b Tile) int {
	switch {
	case Less(a, b):
		return -1
	case Less(b, a):
		return 1
	}
	return 0
}

// newSet renvoie les 24 tuiles mélangées, avec des repères mélangés eux aussi.
func newSet(rng *rand.Rand) []Tile {
	ids := rng.Perm(2 * (MaxNum + 1))
	tiles := make([]Tile, 0, len(ids))
	for c := Black; c <= White; c++ {
		for n := 0; n <= MaxNum; n++ {
			tiles = append(tiles, Tile{ID: ids[len(tiles)], Color: c, Num: n})
		}
	}
	rng.Shuffle(len(tiles), func(i, j int) { tiles[i], tiles[j] = tiles[j], tiles[i] })
	return tiles
}

// insert range une tuile à sa place.
func insert(row []Tile, t Tile) []Tile {
	i, _ := slices.BinarySearchFunc(row, t, compare)
	return slices.Insert(row, i, t)
}

func hidden(row []Tile) int {
	n := 0
	for _, t := range row {
		if !t.Revealed {
			n++
		}
	}
	return n
}
