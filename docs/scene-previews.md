# Whole-scene previews

In the script editor or an assigned scene in Recording studio, choose **Generate
scene preview**. Once ready, the player streams one MP3 assembled on the server.
Choose **Update scene preview** after someone else saves a take. Local script or
take changes clear the displayed preview so it can be regenerated.

Each line uses the currently assigned actor's preferred take for the current line
revision, or their latest current-revision take if none is preferred. If there is
no matching take, offline eSpeak NG speaks the dialogue. Characters receive a
consistent English voice variant automatically. These basic synthetic voices are
placeholders; the Target voice field remains reserved for future voice conversion.
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
Previews support up to 120 lines and 20 minutes total (five minutes per line).
One scene is generated at a time, with an 80-second render deadline. If a long
scene exceeds that deadline, split it into smaller scenes. No partial audio is
published on failure. Playback never auto-starts and is unavailable while an
actor is recording or has an unsaved take.
