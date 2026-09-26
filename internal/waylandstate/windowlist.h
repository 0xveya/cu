#ifndef CU_WINDOWLIST_H
#define CU_WINDOWLIST_H

struct cu_window {
    char *app_id;
    char *title;
    struct cu_window *next;
};

// Returns 0 on success, 2 when the compositor lacks the protocol.
int cu_windowlist(const char *socket_path, struct cu_window **result);
void cu_windowlist_free(struct cu_window *windows);
int cu_screen_size(const char *socket_path, int *width, int *height);

#endif
