from pathlib import Path

path = Path("docs/openapi.yaml")
text = path.read_text()

routes = '''  /comics/{id}:
    get:
      summary: List naturally ordered image pages in a CBZ comic.
      security:
        - sessionAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Comic page manifest.
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ComicManifest"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "404":
          $ref: "#/components/responses/NotFound"
        "415":
          $ref: "#/components/responses/UnsupportedMedia"
  /comics/{id}/{page}:
    get:
      summary: Stream one naturally ordered image page from a CBZ comic.
      security:
        - sessionAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: page
          in: path
          required: true
          schema:
            type: integer
            minimum: 0
      responses:
        "200":
          description: Requested comic page image.
          content:
            image/jpeg:
              schema:
                type: string
                format: binary
            image/png:
              schema:
                type: string
                format: binary
            image/gif:
              schema:
                type: string
                format: binary
            image/webp:
              schema:
                type: string
                format: binary
        "400":
          $ref: "#/components/responses/BadRequest"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "404":
          $ref: "#/components/responses/NotFound"
        "415":
          $ref: "#/components/responses/UnsupportedMedia"
'''

schemas = '''    ComicPage:
      type: object
      required: [index, name, url]
      properties:
        index:
          type: integer
          minimum: 0
        name:
          type: string
        url:
          type: string
    ComicManifest:
      type: object
      required: [pages]
      properties:
        pages:
          type: array
          items:
            $ref: "#/components/schemas/ComicPage"
'''

if "  /comics/{id}:\n" not in text:
    anchor = "  /files/tags:\n"
    if anchor not in text:
        raise SystemExit("comic route insertion anchor missing")
    text = text.replace(anchor, routes + anchor, 1)

if "    ComicManifest:\n" not in text:
    anchor = "    TagMutationRequest:\n"
    if anchor not in text:
        raise SystemExit("comic schema insertion anchor missing")
    text = text.replace(anchor, schemas + anchor, 1)

path.write_text(text)
