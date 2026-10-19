// Submits the analyze form with fetch and swaps the returned content into <main> without a reload; without JavaScript, or if the request fails, the form posts normally.
(() => {
  const main = document.querySelector("main");
  if (!main || !window.fetch) return;

  // listen on <main> so forms in swapped-in content are handled too
  main.addEventListener("submit", async (event) => {
    const form = event.target;
    if (!form.matches("form.analyze")) return;
    event.preventDefault();

    const button = form.querySelector("button[type=submit]");
    button.disabled = true;
    button.classList.add("loading"); // shows the spinner
    button.textContent = "Analyzing…";
    main.setAttribute("aria-busy", "true");

    try {
      const res = await fetch(form.action, {
        method: "POST",
        body: new URLSearchParams(new FormData(form)),
        // the server answers with just the page content instead of the whole layout
        headers: { "X-Partial": "1" },
      });
      if (!(res.headers.get("Content-Type") || "").startsWith("text/html")) throw new Error("unexpected response");
      // error pages come back as HTML content too, with the status and message in them
      main.innerHTML = await res.text();
      main.removeAttribute("aria-busy");
      // tell screen readers the content changed, and start the reader at the top of it
      const heading = main.querySelector("h1");
      if (heading) {
        heading.tabIndex = -1;
        heading.focus();
      }
      window.scrollTo(0, 0);
    } catch {
      form.submit(); // does not fire "submit" again, so this is a plain full-page post
    }
  });
})();
