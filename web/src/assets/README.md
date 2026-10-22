# Map data and artwork

- `world-110m.json`: land/country geometry from `world-atlas@2.0.2`, originally derived from Natural Earth 4.1.0, at 1:110m resolution. Used for the coarse offline basemap, 3D country outlines, and a coarse display-coordinate region mask. https://github.com/topojson/world-atlas
- Natural Earth data is public domain: https://www.naturalearthdata.com/about/terms-of-use/
- world-atlas redistribution license is included in `world-atlas-LICENSE.txt`.
- `favicon.svg`: reused from this repository's original Footprint icon.

The city names on the offline basemap are geographic reference labels, not photo locations or records of visits. Actual memories use separate travel, food and couple pins. `../chinaBoundary.ts` is decoded from the China country geometry in the same bundled dataset, with a small coastal tolerance for display conversion.
