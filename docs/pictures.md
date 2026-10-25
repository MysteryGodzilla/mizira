# Pictures

How a picture request in chat becomes a posted image, for whoever changes it next. Every setting is
listed at the top of `plugins/imagegen.py`; this page covers the flow, the order of its steps, and why
that order matters.

## Parts

| Part | Where | Job |
|---|---|---|
| Chat model | | Calls `generate_image` with `prompt`, and optionally `style`, `image_url` (an edit) or `reference_url` |
| People wrapper | `internal/irc/tools_picturepeople.go` | Adds `people` (from memory) and `request` (the asker's own words); corrects `style` |
| Looks | `internal/core/memory_looks.go` | `Look{Appearance, Hair, Outfit, Variants}` per person; `For` picks a variant |
| The tool | `plugins/imagegen.py` | Cleans the prompt, runs the prompt writer, binds looks, builds the workflow, checks safety, uploads |
| Prompt writer | `metald_tools/promptrefine.py` | Expands the prompt (`IMAGE_PROMPT_REFINER`) |
| Backend | `metald_tools/comfyui.py`, `runpod.py` | A local ComfyUI, or a RunPod serverless worker-comfyui endpoint |
| Worker | separate image | ComfyUI with the Qwen Image models; downloads the LoRAs a job lists (`input.loras`) once, sha256-checked |

## A new picture

### In Go (`PicturePeopleTool.Execute`)

1. `request` is the asker's own words. "me"/"myself" become the asker's nick when they're in the
   picture (`speakerNamed`), and the bot's name is dropped once, where it was used to address her
   (`selfNamed`; said twice running, "mizira mizira draw ...", both go).
2. Style: if the request names a style, it replaces whatever the model chose (`namedStyle`). A draw
   style the request doesn't name is dropped (`unaskedDrawStyle`), since the model carries the last
   picture's style over.
3. Retries: a request that names a known person is kept for an hour, per conversation and asker
   (`lastAsked`). A later request that names nobody ("try that again") gets the kept one in front of it.
4. `unaskedSelf` drops the bot from the prompt when the request names others but not her. If the
   asker is in the request but not the prompt, she wrote herself in for "me", so the asker's name
   replaces hers.
5. `PicturePeople` lists everyone known in the prompt or request (up to 10). Each entry carries:
   - `drawn_as` and `aliases`.
   - `look`: appearance, plus the hair and outfit, or a variant's.
   - `usual`: the default outfit, when a variant replaced it.
   - `notes`: their look memories, when no profile look is set.

   A variant is taken from the request first, then the earlier request, then the prompt. From the
   prompt, it only counts within 4 words of the person's name, because further away the word was
   part of the scene, and not when the word is in their default outfit ("a rat chef" for someone
   whose own outfit is a chef's jacket isn't their chef alt).

### In Python (`draw`)

1. `review_prompt` checks `IMAGE_PROMPT_POLICY` before anything else.
2. Name fixes, for every picture:
   - `kind_named`: "the capybara man" away from the name becomes the name.
   - `one_name`: aliases become one name.
   - `named_first`: "woman (alice)" becomes "alice (woman)".
   - `brackets_dropped`: removes the model's own description in brackets.
3. Clothes, for new pictures only:
   - `look_echo_dropped`: removes clauses that copy the look of the person being described. The look
     goes in once, at the name; a copy made the prompt writer draw that description as a second person.
   - `unasked_clothes_dropped`: removes clothes and hairdos the model made up and the asker never named.
     A clause with a role the asker put before a name (`themes`: "volleyball bob") stays.
   - `outfit_from_request`: clothes the asker named replace the set outfit ("bob in a raincoat").
   - `COSTUME` ("as a pirate", "dressed up as", "cosplay") → `outfit_from_prompt`: the costume the
     model wrote is the one that was asked for. `outfit_from_theme` does the same for a role.
   - `worn_dropped`: removes "wearing ..." from anyone whose set outfit stands. Within it,
     `usual_echo_dropped` removes the default outfit when the model copied it next to a variant.
4. `refined_with_names`: the prompt writer expands the prompt. An art style the asker named that
   isn't a configured style (`asked_style`: "soft pastel style") reaches it like a style, without a
   LoRA; otherwise it draws its default style. If it drops someone's name, it is asked
   once more; after that, the prompt is used as written.
5. `bind_looks` puts each look right after its person's first mention. `IMAGE_PROMPT_CHARACTERS`
   lists anyone the prompt doesn't name.
6. `group_line` adds a count and order for three or more people, then the style trigger goes in front.
7. The workflow is built and run. The safety check follows: the encoder in the same job, or vision
   afterwards. Then the image is uploaded.

**The clothes rule:** a saved outfit (default or variant) is used unless the asker named other
clothes. The models often invent clothes or copy them from the last picture. When two outfits
conflict, the prompt writer adds a second person to wear the other one. Most of steps 2 and 3
prevent that "double", so check a change against `test_imagegen_runpod.py` before you reorder them.

## Edits

- `image_url` is an earlier picture. Its host must be in `IMAGE_EDIT_HOSTS`.
- An edit is a short instruction (`edit_prompt`), never expanded, because expanding it redraws the
  whole scene.
- `dressed()` spells out a saved outfit for "back in her default outfit".
- An edit style's trigger is the instruction, as its LoRA was trained; `{prompt}` is replaced by the request.
- `edit2` styles (admins only) take a second picture, `reference_url`. With `IMAGE_REFERENCE_PROMPT`
  set, the encoder describes that picture and the description fills `{prompt}`; the chat model never sees it.
- `IMAGE_EDIT_CHECK_PROMPT`: the encoder answers YES or NO on whether the change shows. On a NO, the
  edit is tried once more with a new seed.

## Styles (LoRAs)

- One `IMAGE_STYLES` line per style: `name | draw/edit/edit2 | strength [cfg N] | trigger | description | url | sha256 [| guide]`.
- The LoRA file is named by its hash (`style_lora`), so changing a style makes a new file. The worker
  downloads it on first use, from Civitai (with the endpoint's `CIVITAI_API_KEY`), Hugging Face or an
  `EXTRA_LORA_HOSTS` host.
- For a draw style, the trigger goes in front of the prompt. The description reaches the prompt
  writer through `IMAGE_PROMPT_STYLE`, and `guide` is the LoRA's own prompt advice.
- Before adding one, compare it with the base model at the same seed: at the LoRA's recommended
  strength and one stronger, on a person, a pair and a scene. The 8-step turbo model weakens most
  styles, and a LoRA trained on a fine-tune may do little on the base model.

## Looks

- Set in the operator page (Memories, profile). Appearance goes in word for word. Hair and Outfit are
  the defaults. Up to 8 variants each swap the hair, the outfit or both.
- A name the image model would draw ("...Godzilla" gave a tail) can be swapped for a name in pictures
  (`PictureName`). `picture_names` changes it in the final prompt only, so every other step still
  finds the person by their name.
- The defaults have a name too (`DefaultName`, "casual" when blank), and "default", "normal",
  "usual" and "regular" also name them. Named, they win over an alt carried over from an earlier
  request or written into the prompt ("casual hori" after a run of "race hori").
- `Look.For` picks the variant (or the defaults) mentioned nearest a name. On a tie, a mention just
  after the name beats one just before it. Names of 5 or more letters still match with two letters swapped.
- Don't name copyrighted characters in a look: the model draws that character instead.

## Testing

- `python -m unittest discover -s plugins/tests`: `test_imagegen_runpod.py` runs `draw` end to end
  with a fake backend and covers every step above.
- `go test -race -count=1 ./internal/irc ./internal/core`: the people wrapper and `Look.For`.
- Real pictures: the tool's log lines (`image_gen`: `asked for`, `people`, `style`, `prompt`, `edit`,
  `encoder safety verdict`) show each stage. Run an old and a new build on the same logged prompts with
  the same seed, and compare the images.

## Known limits

- About seven people per picture before they start to fuse.
- One picture per request (`OncePerRequestTool`). When one turn makes several new-picture calls ("a
  character sheet of you doing tasks" as one call per task), `mergePictureCalls` (internal/llm) folds
  them into the first as one character sheet, and the others answer `Merged:`.
- Several looks of one person in one picture (a lineup of their alts) go in one prompt, so outfits can
  bleed between them.
- Borrowing someone else's variant ("me in bob's race look") isn't supported.
