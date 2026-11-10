package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"strings"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/frontend/genproto"
)

// Opciones válidas del perfil de mascota. Es la única fuente: el servidor las usa para
// validar el perfil y el formulario las recibe como JSON (static/js/pet-profile.js).
// Deben coincidir con los atributos de src/productcatalogservice/products.json
// (lo comprueba TestPetProfileOptionsMatchCatalog).

type petOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type petTypeOptions struct {
	Ages  []petOption `json:"ages"`
	Sizes []petOption `json:"sizes"`
}

var petProfileOptions = map[string]petTypeOptions{
	"dog": {
		Ages:  []petOption{{"puppy", "Puppy"}, {"adult", "Adult"}, {"senior", "Senior"}},
		Sizes: []petOption{{"small", "Small"}, {"medium", "Medium"}, {"large", "Large"}},
	},
	"cat": {
		Ages:  []petOption{{"kitten", "Kitten"}, {"adult", "Adult"}, {"senior", "Senior"}},
		Sizes: []petOption{{"small", "Small"}, {"medium", "Medium"}},
	},
	"mouse": {
		Ages:  []petOption{{"young", "Young"}, {"adult", "Adult"}},
		Sizes: []petOption{{"small", "Small"}},
	},
}

// normalizePetProfile descarta los valores que no corresponden a la mascota elegida
// (por ejemplo, "dog" + "kitten"), que dejarían la tienda sin productos.
func normalizePetProfile(petType, petAge, petSize string) (string, string, string) {
	options, ok := petProfileOptions[petType]
	if !ok {
		return "", "", ""
	}
	if !hasPetOption(options.Ages, petAge) {
		petAge = ""
	}
	if !hasPetOption(options.Sizes, petSize) {
		petSize = ""
	}
	return petType, petAge, petSize
}

func hasPetOption(options []petOption, value string) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

var petTypeLabels = map[string]string{"dog": "Dog", "cat": "Cat", "mouse": "Mouse"}

var petEmojis = map[string]string{"dog": "🐶", "cat": "🐱", "mouse": "🐭"}

// petEmoji devuelve el ícono de una mascota para las plantillas.
func petEmoji(petType string) string {
	if emoji, ok := petEmojis[petType]; ok {
		return emoji
	}
	return "🐾"
}

// petLabel devuelve el texto de una mascota, edad o tamaño ("puppy" → "Puppy").
func petLabel(value string) string {
	if label, ok := petTypeLabels[value]; ok {
		return label
	}
	for _, options := range petProfileOptions {
		for _, option := range options.Ages {
			if option.Value == value {
				return option.Label
			}
		}
		for _, option := range options.Sizes {
			if option.Value == value {
				return option.Label
			}
		}
	}
	return value
}

// catalogAds deja solo los anuncios que llevan a productos del catálogo actual. Los del
// adservice original apuntan a productos de Online Boutique que ya no existen y terminaban
// en una página de error.
func catalogAds(ads []*pb.Ad, catalog []*pb.Product) []*pb.Ad {
	var valid []*pb.Ad
	for _, ad := range ads {
		id := strings.TrimPrefix(ad.GetRedirectUrl(), "/product/")
		for _, p := range catalog {
			if p.GetId() == id {
				valid = append(valid, ad)
				break
			}
		}
	}
	return valid
}

// petAd arma un anuncio de un producto real del catálogo, de preferencia para la mascota del perfil.
func petAd(catalog []*pb.Product, petType, petAge, petSize string) *pb.Ad {
	var candidates []*pb.Product
	for _, p := range catalog {
		if productMatchesPetProfile(p, petType, petAge, petSize) {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		candidates = catalog
	}
	if len(candidates) == 0 {
		return nil
	}
	p := candidates[rand.Intn(len(candidates))]
	emoji := "🐾"
	if types := p.GetPetTypes(); len(types) == 1 {
		emoji = petEmoji(types[0])
	}
	return &pb.Ad{
		RedirectUrl: "/product/" + p.GetId(),
		Text:        fmt.Sprintf("%s %s: %s", emoji, p.GetName(), p.GetDescription()),
	}
}

const maxRecommendations = 4

// petRecommendations pone primero las recomendaciones para las mascotas indicadas y completa
// con productos del catálogo para esas mascotas, sin repetir ni incluir los excluidos (el producto
// que se está viendo o lo que ya está en el carrito). Si faltan, completa con el resto de las
// recomendaciones. Sin mascotas, deja las recomendaciones como vienen.
func petRecommendations(recs, catalog []*pb.Product, petTypes, exclude []string) []*pb.Product {
	if len(petTypes) == 0 {
		return recs
	}
	seen := make(map[string]bool)
	for _, id := range exclude {
		seen[id] = true
	}
	var out []*pb.Product
	add := func(p *pb.Product) {
		if len(out) < maxRecommendations && !seen[p.GetId()] {
			seen[p.GetId()] = true
			out = append(out, p)
		}
	}
	forPet := func(p *pb.Product) bool {
		for _, petType := range petTypes {
			if containsString(p.GetPetTypes(), petType) {
				return true
			}
		}
		return false
	}
	for _, p := range recs {
		if forPet(p) {
			add(p)
		}
	}
	for _, p := range catalog {
		if forPet(p) {
			add(p)
		}
	}
	for _, p := range recs {
		add(p)
	}
	return out
}

// recommendationPetTypes devuelve las mascotas para las recomendaciones: la del perfil o, si no
// hay perfil, las de los productos que se están viendo.
func recommendationPetTypes(r *http.Request, products []*pb.Product) []string {
	if petType, _, _ := getPetProfile(r); petType != "" {
		return []string{petType}
	}
	var petTypes []string
	for _, p := range products {
		for _, petType := range p.GetPetTypes() {
			if !containsString(petTypes, petType) {
				petTypes = append(petTypes, petType)
			}
		}
	}
	return petTypes
}
