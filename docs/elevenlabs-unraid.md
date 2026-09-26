# ElevenLabs character voices

## Connect the account

In your ElevenLabs account, create an API key with access to **Voices** (read)
and **Voice Changer**. In Unraid, edit the StoryForge container and add a Variable:

| Name and Key | Value |
| --- | --- |
| `ELEVENLABS_API_KEY` | Your ElevenLabs API key |

Keep this key in the container configuration. Do not put it in scripts, source
control, or frontend configuration. Apply the setting and update StoryForge.
No additional container or volume is needed. Without a key, recording, raw
playback, and offline scene previews continue to work.

## Assign voices

Open **Cast & characters**, select a **Target voice** for each character, and
click **Save**. Existing free-text voice labels are retained as a reminder, but
must be replaced with a real account voice. Assignments store the unique voice
ID, so renaming an ElevenLabs voice does not disconnect it.

The server loads the account's voice list on demand and caches it for ten minutes.
After creating a voice in ElevenLabs, click **Refresh voices** beside a character
to bypass the cache. A failed refresh leaves existing assignments intact. Before
queuing paid conversions, the server checks the voice list again and rejects
missing/deleted voices.

## Voice a completed scene

Every line must have a current take from its assigned actor and a target voice.
New recordings automatically become preferred. Admins can listen to all takes
under **Review takes** and mark an older take preferred; actors cannot change
that selection manually. A subsequent new recording becomes preferred again.

In the script editor or admin recording studio, choose **Voice scene** and confirm
the upload/credit notice. StoryForge snapshots the selected takes and voice IDs,
queues each line, and processes one paid request at a time. The queue continues
when the browser closes. Voice Changer uses `eleven_multilingual_sts_v2`, MP3
44.1 kHz/128 kbps output, stability 0.5, similarity 0.75, style 0, speaker boost on,
and noise removal off. A temporary mono WAV copy is sent to ElevenLabs; original
recordings are never replaced. Audio is uploaded to ElevenLabs only when an admin
starts a conversion, not when an actor saves a take.

When all lines complete, StoryForge assembles one scene MP3, in script order with
0.3 seconds between lines. Admins and actors assigned to the scene can listen to
it. Actors also see their selected line's latest converted recording, with their
raw take history still available separately.

**Regenerate line** is admin-only. It makes one new paid conversion, reuses the
other completed lines, and rebuilds the scene. If other lines also need updating,
use **Voice scene** first. Matching successful conversions are reused, including
after a partially failed run. New takes, changed voices, script edits or reordering
mark the scene outdated; they never automatically spend credits. Earlier completed
audio remains playable while new conversion is pending or failed.

## Failures, restart, and storage

The queue and progress are stored in PostgreSQL. Waiting jobs resume on restart.
If a paid request is interrupted, its outcome may be uncertain: the job stops
with a message instead of being sent again automatically. Check your ElevenLabs
history/credits, then explicitly retry with **Voice scene** or **Regenerate line**.
Rate-limit and credit errors also stop the run rather than retrying charges in a loop.

Converted files (`converted-*.mp3`) and compiled scenes (`voiced-scene-*.mp3`) live
on the existing recordings volume. They are persistent derived recordings, not
the disposable offline preview cache. Keep backups of both the volume and the
database. Prior successful conversions are retained for playback/reuse.

Scene limits match offline previews: up to 120 lines and 20 minutes compiled.
Each source take is limited to five minutes. Very long scenes may need splitting.

Integration tests use a disposable fake provider with a dummy key. Never set
`ELEVENLABS_TEST_URL` on your production container; it is rejected outside the
explicit localhost development-authentication mode.
