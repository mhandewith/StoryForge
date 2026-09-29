# Recording, reviewing, and exporting

## Record a take

Recording starts after a visible **3, 2, 1** countdown, once microphone permission
has been granted. **Cancel countdown** returns to the previous take without
recording. **Stop recording** lets you listen before saving.

**Nailed it** stops and saves the recording, then opens the next assigned line.
You can also use it after stopping and listening. The next line does not start
recording automatically. After the last line, you return to the scene list.
If saving fails, the recording remains available for playback, download, or retry;
the studio stays on the same line.

## Record as a team (administrator)

Open **Recording studio → Team mode**, name a team, and select at least two
actors. Saved teams can be selected and edited on later visits. The studio shows
the team's assigned lines in script order, including Any actor roles, and labels
each line with the actor and character. For an Any actor role, select the team
member who will perform it. Takes are saved under that actor's identity.

## Review and process (administrator)

Open **Review takes** and select a project and scene. Each line has a raw-take
selector/player, a preferred-take button, conversion status, and converted audio
when available. Choosing a take for playback does not change the preferred take.
Use **Make preferred** to choose it for processing.

**Process line** is available for an unprocessed line or when the preferred take
or target voice differs from its latest conversion. It requires a current recorded
take and target voice, and asks for confirmation before using ElevenLabs credits.
The previous conversion remains playable while the new one is processed.

Use **Refresh Voices** beside **Reload** in the admin header after creating a
voice in ElevenLabs. Character voice selectors update from the refreshed list.
The button is also available in Review Takes.

## Export audio (administrator)

New ElevenLabs voice conversions and isolation results automatically trim quiet
audio from the beginning and end, keeping about 100 ms around the performance.
Pauses within the line stay intact. Raw recordings are untouched. Very quiet or
silent results are kept intact when no signal exceeds the conservative -50 dBFS
threshold. Scene timing uses the processed file's duration.

Existing conversions are unchanged; this applies when a new conversion finishes.
Trimming itself runs locally and makes no additional ElevenLabs requests.

**Export project audio** is available in the script workspace and Review Takes.
It downloads a ZIP containing one file per recorded line, grouped in numbered
scene folders. The scene and line numbers are zero-padded so names sort in script
order, for example:

```text
0000000001_The-forest/
  0000000001_Wolf_Hannah_converted.mp3
  0000000002_Owl_Hazel_raw.webm
```

The export chooses the latest completed conversion matching the current
preferred take and target voice. If there is no matching conversion, it includes
the preferred raw recording. Original file formats are preserved without another
encoding step. No placeholder text-to-speech or paid processing is triggered.
`manifest.json` records dialogue, actors, take IDs, filenames, and missing lines.
Missing lines are listed but have no audio file. Exports are limited to 512 MB of
audio and one export at a time.

## Edit dialogue

Click a line's number in Scenes & script. Its editor opens directly under that
line. Save changes or cancel to close it.
