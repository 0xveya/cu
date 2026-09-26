#include "windowlist.h"
#include "toplevel-protocol.h"

#include <stddef.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <sys/time.h>
#include <unistd.h>

struct cu_state {
    struct ext_foreign_toplevel_list_v1 *list;
    struct cu_window *head;
    struct cu_window *tail;
    int failed;
};

static void handle_closed(void *data, struct ext_foreign_toplevel_handle_v1 *handle) {
    (void)data;
    (void)handle;
}

static void handle_done(void *data, struct ext_foreign_toplevel_handle_v1 *handle) {
    (void)data;
    (void)handle;
}

static void handle_title(void *data, struct ext_foreign_toplevel_handle_v1 *handle, const char *title) {
    (void)handle;
    struct cu_window *window = data;
    free(window->title);
    window->title = strdup(title);
}

static void handle_app_id(void *data, struct ext_foreign_toplevel_handle_v1 *handle, const char *app_id) {
    (void)handle;
    struct cu_window *window = data;
    free(window->app_id);
    window->app_id = strdup(app_id);
}

static void handle_identifier(void *data, struct ext_foreign_toplevel_handle_v1 *handle, const char *identifier) {
    (void)data;
    (void)handle;
    (void)identifier;
}

static const struct ext_foreign_toplevel_handle_v1_listener handle_listener = {
    .closed = handle_closed,
    .done = handle_done,
    .title = handle_title,
    .app_id = handle_app_id,
    .identifier = handle_identifier,
};

static void list_toplevel(void *data, struct ext_foreign_toplevel_list_v1 *list,
                          struct ext_foreign_toplevel_handle_v1 *handle) {
    (void)list;
    struct cu_state *state = data;
    struct cu_window *window = calloc(1, sizeof(*window));
    if (window == NULL) {
        state->failed = 1;
        return;
    }
    if (state->tail == NULL) {
        state->head = window;
    } else {
        state->tail->next = window;
    }
    state->tail = window;
    ext_foreign_toplevel_handle_v1_add_listener(handle, &handle_listener, window);
}

static void list_finished(void *data, struct ext_foreign_toplevel_list_v1 *list) {
    (void)data;
    (void)list;
}

static const struct ext_foreign_toplevel_list_v1_listener list_listener = {
    .toplevel = list_toplevel,
    .finished = list_finished,
};

static void registry_global(void *data, struct wl_registry *registry, uint32_t name,
                            const char *interface, uint32_t version) {
    struct cu_state *state = data;
    if (strcmp(interface, ext_foreign_toplevel_list_v1_interface.name) != 0 || state->list != NULL) {
        return;
    }
    uint32_t bind_version = version < 1 ? version : 1;
    state->list = wl_registry_bind(registry, name, &ext_foreign_toplevel_list_v1_interface, bind_version);
    if (state->list != NULL) {
        ext_foreign_toplevel_list_v1_add_listener(state->list, &list_listener, state);
    }
}

static void registry_removed(void *data, struct wl_registry *registry, uint32_t name) {
    (void)data;
    (void)registry;
    (void)name;
}

static const struct wl_registry_listener registry_listener = {
    .global = registry_global,
    .global_remove = registry_removed,
};

void cu_windowlist_free(struct cu_window *windows) {
    while (windows != NULL) {
        struct cu_window *next = windows->next;
        free(windows->app_id);
        free(windows->title);
        free(windows);
        windows = next;
    }
}

static struct wl_display *cu_connect(const char *socket_path) {
    struct sockaddr_un address = {.sun_family = AF_UNIX};
    if (strlen(socket_path) >= sizeof(address.sun_path)) {
        return NULL;
    }
    strcpy(address.sun_path, socket_path);
    int fd = socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if (fd < 0) {
        return NULL;
    }
    struct timeval timeout = {.tv_sec = 3};
    setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof(timeout));
    setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &timeout, sizeof(timeout));
    if (connect(fd, (struct sockaddr *)&address, sizeof(address)) != 0) {
        close(fd);
        return NULL;
    }
    struct wl_display *display = wl_display_connect_to_fd(fd);
    if (display == NULL) {
        close(fd);
        return NULL;
    }
    return display;
}

int cu_windowlist(const char *socket_path, struct cu_window **result) {
    *result = NULL;
    struct wl_display *display = cu_connect(socket_path);
    if (display == NULL) {
        return 1;
    }
    struct cu_state state = {0};
    struct wl_registry *registry = wl_display_get_registry(display);
    if (registry == NULL) {
        wl_display_disconnect(display);
        return 1;
    }
    wl_registry_add_listener(registry, &registry_listener, &state);
    int status = 0;
    if (wl_display_roundtrip(display) < 0) {
        status = 1;
    } else if (state.list == NULL) {
        status = 2;
    } else if (wl_display_roundtrip(display) < 0 || state.failed) {
        status = 1;
    }
    wl_display_disconnect(display);
    if (status != 0) {
        cu_windowlist_free(state.head);
        return status;
    }
    *result = state.head;
    return 0;
}

struct cu_screen {
    int width;
    int height;
};

static void screen_geometry(void *data, struct wl_output *output, int32_t x, int32_t y,
                            int32_t physical_width, int32_t physical_height, int32_t subpixel,
                            const char *make, const char *model, int32_t transform) {
    (void)data;
    (void)output;
    (void)x;
    (void)y;
    (void)physical_width;
    (void)physical_height;
    (void)subpixel;
    (void)make;
    (void)model;
    (void)transform;
}

static void screen_mode(void *data, struct wl_output *output, uint32_t flags,
                        int32_t width, int32_t height, int32_t refresh) {
    (void)output;
    (void)refresh;
    struct cu_screen *screen = data;
    if (flags & WL_OUTPUT_MODE_CURRENT) {
        screen->width = width;
        screen->height = height;
    }
}

static const struct wl_output_listener screen_listener = {
    .geometry = screen_geometry,
    .mode = screen_mode,
};

static void screen_global(void *data, struct wl_registry *registry, uint32_t name,
                          const char *interface, uint32_t version) {
    if (strcmp(interface, wl_output_interface.name) != 0) {
        return;
    }
    (void)version;
    struct wl_output *output = wl_registry_bind(registry, name, &wl_output_interface, 1);
    if (output != NULL) {
        wl_output_add_listener(output, &screen_listener, data);
    }
}

static const struct wl_registry_listener screen_registry_listener = {
    .global = screen_global,
    .global_remove = registry_removed,
};

int cu_screen_size(const char *socket_path, int *width, int *height) {
    struct wl_display *display = cu_connect(socket_path);
    if (display == NULL) {
        return 1;
    }
    struct cu_screen screen = {0};
    struct wl_registry *registry = wl_display_get_registry(display);
    if (registry == NULL) {
        wl_display_disconnect(display);
        return 1;
    }
    wl_registry_add_listener(registry, &screen_registry_listener, &screen);
    int status = wl_display_roundtrip(display) < 0 || wl_display_roundtrip(display) < 0 ||
                 screen.width <= 0 || screen.height <= 0;
    wl_display_disconnect(display);
    if (status != 0) {
        return 1;
    }
    *width = screen.width;
    *height = screen.height;
    return 0;
}
