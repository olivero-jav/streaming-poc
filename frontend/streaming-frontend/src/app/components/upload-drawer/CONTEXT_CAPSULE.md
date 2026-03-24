# CONTEXT CAPSULE - upload-drawer

## Purpose
Collect metadata and file selection for `POST /videos` upload.

## Contract
- Required:
  - `title`
  - `file`
- Optional:
  - `description`

## Interaction Notes
- Dropzone is a button that triggers hidden file input (`openFilePicker`).
- Submit enablement is computed by method `canSubmit()`:
  - form valid
  - file selected
- On close, parent resets drawer form through `resetForm()`.

## Styling Notes
- Typography should stay on shared app scale tokens (`--fs-xs/sm/md`).
- Drawer is intentionally dark and compact to match admin VOD page.
