(function () {
  fetch("/versions.json")
    .then(function (r) { return r.json(); })
    .then(function (data) {
      var segments = window.location.pathname.split("/").filter(Boolean);
      var current = segments[0] || data.latest;

      var select = document.createElement("select");
      select.className = "lg-version-select";
      select.setAttribute("aria-label", "Documentation version");

      data.versions.forEach(function (v) {
        var opt = document.createElement("option");
        opt.value = v.path;
        opt.textContent = v.label;
        if (v.path === current) {
          opt.selected = true;
        }
        select.appendChild(opt);
      });

      select.addEventListener("change", function () {
        window.location.href = "/" + select.value + "/";
      });

      var target = document.querySelector("#menu-bar .right-buttons")
        || document.querySelector("#menu-bar-hover-placeholder .right-buttons");
      if (target) {
        target.insertBefore(select, target.firstChild);
      } else {
        // Fallback for pages without the standard menu bar (shouldn't
        // normally happen with the current theme, but fail visibly
        // rather than silently drop the selector).
        var wrap = document.createElement("div");
        wrap.style.cssText = "position:fixed;top:8px;right:8px;z-index:999;";
        wrap.appendChild(select);
        document.body.appendChild(wrap);
      }
    })
    .catch(function () {
      // versions.json missing/unreachable — don't break the page over a
      // missing selector.
    });
})();
