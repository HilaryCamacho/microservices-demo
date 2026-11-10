package main

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
