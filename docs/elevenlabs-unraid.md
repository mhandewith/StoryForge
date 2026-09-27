# ElevenLabs character voices

## Connect the account

In your ElevenLabs account, create an API key with access to **Voices** (read)
and **Voice Changer**, plus **Audio Isolation** when using isolation. In Unraid,
edit the StoryForge container and add a Variable:

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

For guest actors or anyone keeping their own voice, select **Isolation — keep
original voice** and save. This sends the preferred raw take to ElevenLabs Audio
Isolation instead of Voice Changer. Isolation removes background noise while
retaining the performer’s voice; it also uses credits. A scene can mix both modes,
and the server compiles their results together. The same admin-only regeneration,
cache reuse, and original recording preservation apply. Changing between isolation
and a character voice marks the existing result outdated. No new Unraid variable
is needed; enable Audio Isolation on the existing API key.

The server loads the account's voice list on demand and caches it for ten minutes.
After creating a voice in ElevenLabs, click **Refresh voices** beside a character
to bypass the cache. A failed refresh leaves existing assignments intact. Before
queuing paid conversions, the server checks the voice list again and rejects
missing/deleted voices.

## Voice a completed scene

Every line must have a current take from its assigned actor and a target voice.
Choose **Any actor** under **Played by** to offer a role to every linked actor.
Each performer can record and replay their own takes. Admins can compare everyone's
performances under **Review takes** and mark one preferred across all actors.
Previews and ElevenLabs use that selection. The newest upload becomes preferred
by default, including uploads after an admin selection. Assigning the role back to
one actor restricts new recordings and selects only that actor's eligible takes;
other recordings remain saved. Unassigned roles are not open to everyone.
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

**Convert line** and **Regenerate line** are admin-only, under **Converted lines**
or the selected line in the admin recording studio. Only that line needs a current
take and target voice. Each line also has a **Recorded takes** dropdown and raw
audio player for auditioning every actor's takes. Selecting one only changes
playback; click **Make preferred** to use it for conversion. Earlier script versions
and takes from actors no longer assigned remain playable but cannot be preferred
from this panel. Only admins see this cross-actor take selector.
Only the selected line needs a current
take and target voice; the rest of the scene can be unfinished. Each action makes
one paid conversion (or isolation), without rebuilding the full converted scene.
Choose **Generate scene preview** afterward to hear the result in context. Previews
prefer the latest successful conversion matching the selected take and target voice,
then the raw take, then free computer speech. Changing the take or voice excludes
outdated conversions. Generating previews never starts paid work.
Use **Voice scene** to rebuild a fully converted scene. Matching successful conversions are reused, including
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

Whole-scene ElevenLabs conversions support up to 120 lines and 20 minutes compiled.
Individual lines can be converted in larger scenes too.
Free offline previews automatically split longer scenes into listening parts.
Each source take is limited to five minutes. Very long scenes may need splitting.

Integration tests use a disposable fake provider with a dummy key. Never set
`ELEVENLABS_TEST_URL` on your production container; it is rejected outside the
explicit localhost development-authentication mode.
