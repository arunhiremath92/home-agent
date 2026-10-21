(() => {
  const tokenInput = document.querySelector("#token");
  const connection = document.querySelector("#connection");
  const dashboard = document.querySelector("#dashboard");
  const status = document.querySelector("#status");
  const connectStatus = document.querySelector("#connectStatus");
  const summary = document.querySelector("#summary");
  const signOut = document.querySelector("#signOut");
  const currentLocation = document.querySelector("#currentLocation");
  const locationStatus = document.querySelector("#locationStatus");
  const memoryForm = document.querySelector("#memoryForm");
  let token = sessionStorage.getItem("home-agent-token") || "";

  function message(text, error = false) {
    const target = connection.hidden ? status : connectStatus;
    target.textContent = text;
    target.classList.toggle("error", error);
  }

  async function request(path, options = {}) {
    const response = await fetch(path, {
      ...options,
      headers: {
        Authorization: `Bearer ${token}`,
        ...(options.body ? { "Content-Type": "application/json" } : {}),
        ...options.headers,
      },
    });
    if (response.status === 401) {
      sessionStorage.removeItem("home-agent-token");
      token = "";
      tokenInput.value = "";
      setConnected(false);
      throw new Error("Access token expired or is not valid for this household. Connect again.");
    }
    if (!response.ok) {
      let detail = `Request failed (${response.status})`;
      try {
        const body = await response.json();
        detail = body.error?.message || detail;
      } catch {}
      throw new Error(detail);
    }
    return response.status === 204 ? null : response.json();
  }

  function setConnected(connected) {
    connection.hidden = connected;
    dashboard.hidden = !connected;
    signOut.hidden = !connected;
  }

  function showItems(selector, values, render) {
    const list = document.querySelector(selector);
    list.replaceChildren();
    if (!values.length) {
      const empty = document.createElement("li");
      empty.className = "empty";
      empty.textContent = "Nothing here yet.";
      list.append(empty);
      return;
    }
    for (const value of values) list.append(render(value));
  }

  function item(title, detail = "") {
    const row = document.createElement("li");
    row.append(document.createTextNode(title));
    if (detail) {
      const sub = document.createElement("span");
      sub.className = "sub";
      sub.textContent = detail;
      row.append(sub);
    }
    return row;
  }

  async function refresh() {
    message("Loading your household data…");
    try {
      const [tasks, notes, shortcuts] = await Promise.all([
        request("/api/v1/tasks"),
        request("/api/v1/notes"),
        request("/api/v1/shortcuts"),
      ]);
      showItems("#tasks", tasks.data, task => item(task.title, task.description || task.status));
      showItems("#notes", notes.data, note => item(note.content, new Date(note.created_at).toLocaleString()));
      showItems("#shortcuts", shortcuts.data, shortcut => {
        const row = item(shortcut.name, shortcut.description);
        const remove = document.createElement("button");
        remove.type = "button";
        remove.className = "remove";
        remove.textContent = "Remove";
        remove.addEventListener("click", async () => {
          try {
            await request(`/api/v1/shortcuts/${encodeURIComponent(shortcut.name)}`, { method: "DELETE" });
            await refresh();
          } catch (error) {
            message(error.message, true);
          }
        });
        row.classList.add("item-row");
        row.append(remove);
        return row;
      });
      message("Connected securely.");
    } catch (error) {
      message(error.message, true);
    }
  }

  async function submitForm(selector, path, build, success) {
    const form = document.querySelector(selector);
    form.addEventListener("submit", async event => {
      event.preventDefault();
      const button = form.querySelector("button[type=submit]");
      button.disabled = true;
      try {
        await request(path, { method: "POST", body: JSON.stringify(build(new FormData(form))) });
        form.reset();
        message(success);
        await refresh();
      } catch (error) {
        message(error.message, true);
      } finally {
        button.disabled = false;
      }
    });
  }

  document.querySelector("#tokenForm").addEventListener("submit", async event => {
    event.preventDefault();
    token = tokenInput.value.trim();
    if (!token) return;
    sessionStorage.setItem("home-agent-token", token);
    setConnected(true);
    await refresh();
  });
  document.querySelector("#refresh").addEventListener("click", refresh);
  currentLocation.addEventListener("click", () => {
    if (!window.isSecureContext) {
      locationStatus.textContent = "Current location requires HTTPS or localhost.";
      locationStatus.classList.add("error");
      return;
    }
    if (!navigator.geolocation) {
      locationStatus.textContent = "This browser does not support location access.";
      locationStatus.classList.add("error");
      return;
    }

    currentLocation.disabled = true;
    locationStatus.textContent = "Waiting for location permission…";
    locationStatus.classList.remove("error");
    navigator.geolocation.getCurrentPosition(
      position => {
        memoryForm.querySelector('[name="latitude"]').value = position.coords.latitude;
        memoryForm.querySelector('[name="longitude"]').value = position.coords.longitude;
        locationStatus.textContent = `Coordinates filled (accuracy about ${Math.round(position.coords.accuracy)} m).`;
        currentLocation.disabled = false;
      },
      error => {
        const messages = {
          1: "Location permission was denied. Allow location access in your browser settings and try again.",
          2: "Your location is unavailable. Check your device's location settings and try again.",
          3: "Location lookup timed out. Try again.",
        };
        locationStatus.textContent = messages[error.code] || "Could not get your current location.";
        locationStatus.classList.add("error");
        currentLocation.disabled = false;
      },
      { enableHighAccuracy: true, timeout: 15000, maximumAge: 0 },
    );
  });
  memoryForm.addEventListener("reset", () => {
    locationStatus.textContent = "";
    locationStatus.classList.remove("error");
  });
  signOut.addEventListener("click", () => {
    sessionStorage.removeItem("home-agent-token");
    token = "";
    tokenInput.value = "";
    setConnected(false);
    message("");
  });

  submitForm("#taskForm", "/api/v1/tasks", data => ({
    title: data.get("title").trim(),
    description: data.get("description").trim(),
  }), "Task added.");
  submitForm("#noteForm", "/api/v1/notes", data => ({
    content: data.get("content").trim(),
  }), "Note saved.");
  submitForm("#shortcutForm", "/api/v1/shortcuts", data => ({
    name: data.get("name").trim(),
    description: data.get("description").trim(),
  }), "Shortcut saved.");
  submitForm("#memoryForm", "/api/v1/location-memories", data => ({
    store_name: data.get("store_name").trim(),
    store_type: data.get("store_type"),
    latitude: Number(data.get("latitude")),
    longitude: Number(data.get("longitude")),
    message: data.get("message").trim(),
  }), "Place memory saved.");

  document.querySelector("#summaryForm").addEventListener("submit", async event => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const query = new URLSearchParams({
      store_name: data.get("store_name").trim(),
      store_type: data.get("store_type"),
    });
    try {
      const result = await request(`/api/v1/location-memories/summary?${query}`);
      summary.textContent = result.data.summary;
      summary.hidden = false;
      message("Summary ready.");
    } catch (error) {
      message(error.message, true);
    }
  });

  if (token) {
    tokenInput.value = token;
    setConnected(true);
    refresh();
  }
})();
