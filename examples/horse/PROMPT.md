# Horse in Calc

Use the stateless `cu` MCP tools to draw a detailed horse in a real isolated
LibreOffice Calc desktop.

On a sheet named `Pen`, use Calc's unfilled freeform drawing tool and held-button
`path` actions. Draw a continuous silhouette, four articulated legs, hooves,
mane, tail, eye, nostril, and anatomical lines. Use smooth curves and black
lines. Inspect native screenshots after meaningful batches.

On a sheet named `Tiles`, draw the horse with tiny square cells and four colors.
Show it at 20% zoom. Raw integer values may be pasted through the desktop
clipboard; select matching values with Calc's Find All and apply backgrounds
through its palette. Hide the values with the `;;;` number format. Do not
import a finished spreadsheet, graphic, SVG, macro, or UNO formatting script.
All drawing and formatting must happen through the application's GUI.

Save `horse.ods`, a screenshot of each sheet, and recordings of both workflows.
Label accelerated playback and cuts explicitly. Leave the saved workbook open
on `Tiles` without dialogs or selected drawing objects.

## Recorded result

- `horse.ods`: 53 native drawing objects on `Pen`; a 180 × 110 cell canvas on
  `Tiles`, with 4,448 colored cells and four tones.
- `horse.png` and `tiles.png`: native 1280 × 720 desktop captures.
- `horse.mp4`: successful pen drawing, 4× playback.
- `tiles.mp4`: real GUI selection, four palette passes, number formatting, and
  saving; 4× playback with cuts between steps.

The tile values and all four background colors were checked in the saved ODS,
then the clean result was inspected in Calc. The four palette passes took
about 5.1 seconds of tool execution after the values were pasted. This excludes
application setup, data preparation, inspection, and input debugging.
