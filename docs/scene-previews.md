# Whole-scene previews

In the script editor or an assigned scene in Recording studio, choose **Generate
scene preview**. Once ready, each player streams an MP3 assembled on the server.
Choose **Update scene preview** after someone else saves a take. Local script or
take changes clear the displayed preview so it can be regenerated.

Each line uses the currently assigned actor's preferred take for the current line
revision, or their latest current-revision take if none is preferred. If there is
no matching take, offline eSpeak NG speaks the dialogue. Characters receive a
consistent English voice variant automatically. These basic synthetic voices are
placeholders; the Target voice field is used separately for ElevenLabs conversion.
Performance directions are displayed to actors, not spoken by the synthesizer.

Lines play sequentially in script order, with a 0.3-second gap. Preview playback
does not use the future timeline's Start time field. Original recordings are never
modified. Different source formats are normalized to mono 24 kHz before assembly.

## Unraid

Update the StoryForge image. No additional container, API key, environment
variable, or volume is needed. The image includes eSpeak NG and FFmpeg. Generated
MP3s live under `/data/recordings/scene-previews` on the existing recordings mount.
These files are disposable caches; original recordings remain in the parent
directory. Different scene versions retain separate cached previews. To reclaim
cache space, stop StoryForge and remove only the `scene-previews` subdirectory;
it is recreated on the next generation request.

Only admins and actors assigned to a scene can generate or play that whole scene,
including the other roles' performances. Playback checks access on every request.
Long previews automatically build numbered listening parts, each containing up to
40 lines. Long recordings produce smaller parts using a ten-minute duration budget.
Each part is a single server-generated audio file; finished parts can be played
while the remaining parts build. The scene itself is not split or changed.
There is no 120-line limit on previews. The audio compiler retains its safety limit
of twenty minutes per part and five minutes per line. Generating again reuses
unchanged parts and includes current takes. If the scene changes during generation,
generate again so that all parts use the same scene version.
One part is generated at a time, with an 80-second render deadline. Completed
parts remain available if a later part fails; generate again to retry, reusing
cached parts. Playback never auto-starts and is unavailable while an
actor is recording or has an unsaved take.
