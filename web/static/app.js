// Submits the form with fetch and swaps the result into <main>; falls back to a normal post.
(() => {
  const main = document.querySelector("main");
  if (!main || !window.fetch) return;

  main.addEventListener("submit", async (event) => {
    const form = event.target;
    if (!form.matches("form.analyze")) return;
    event.preventDefault();

    const button = form.querySelector("button[type=submit]");
    button.disabled = true;
    button.classList.add("loading");
    button.textContent = "Analyzing…";
    main.setAttribute("aria-busy", "true");

    try {
      const res = await fetch(form.action, {
        method: "POST",
        body: new URLSearchParams(new FormData(form)),
        headers: { "X-Partial": "1" },
      });
      if (!(res.headers.get("Content-Type") || "").startsWith("text/html")) throw new Error("unexpected response");
      main.innerHTML = await res.text();
      main.removeAttribute("aria-busy");
      const heading = main.querySelector("h1");
      if (heading) {
        heading.tabIndex = -1;
        heading.focus();
      }
      window.scrollTo(0, 0);
    } catch {
      form.submit(); // does not fire "submit" again
    }
  });
})();
