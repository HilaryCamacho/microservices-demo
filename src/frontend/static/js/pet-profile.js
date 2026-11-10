// Ajusta las edades y los tamaños del formulario según la mascota elegida.
// Lo usan la página de inicio y /profile. Las opciones vienen del servidor
// (petProfileOptions en pet_profile.go) en window.PET_OPTIONS.

function fillPetSelect(select, items) {
  var current = select.value;
  // La primera opción es la vacía de cada página ("Select age", "Any age", ...)
  var placeholder = select.options.length ? select.options[0].text : "";

  select.innerHTML = "";
  select.appendChild(new Option(placeholder, ""));
  items.forEach(function (item) {
    select.appendChild(new Option(item.label, item.value, false, item.value === current));
  });
}

function updatePetOptions() {
  var options = (window.PET_OPTIONS || {})[document.getElementById("pet_type").value];

  fillPetSelect(document.getElementById("pet_age"), options ? options.ages : []);
  fillPetSelect(document.getElementById("pet_size"), options ? options.sizes : []);
}

document.addEventListener("DOMContentLoaded", updatePetOptions);
