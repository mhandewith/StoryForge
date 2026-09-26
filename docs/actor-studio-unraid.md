# Actor studio: Unraid upgrade

The actor studio uses your existing Cloudflare Google login. Each actor's login
email is linked in **Cast & characters**. Administrators are selected by an
environment variable, not by who logs in first.

## 1. Prepare persistent recording storage

In the Unraid terminal, create this directory and give StoryForge's container
user access. These commands change only the new recording directory:

```sh
mkdir -p /mnt/user/appdata/storyforge/recordings
chown 10001:10001 /mnt/user/appdata/storyforge/recordings
chmod 750 /mnt/user/appdata/storyforge/recordings
```

Edit the StoryForge container and add a **Path**:

| Field | Value |
| --- | --- |
| Name | Recordings |
| Container path | `/data/recordings` |
| Host path | `/mnt/user/appdata/storyforge/recordings` |
| Access mode | Read/Write |

## 2. Add the login and storage variables

Add each as a **Variable**. Fill in both the Name and Key fields in Unraid;
the Key must exactly match the name below.

| Key | Value |
| --- | --- |
| `STORYFORGE_PUBLIC_ORIGIN` | `https://storyforge.handewith.family` |
| `CF_ACCESS_TEAM_DOMAIN` | `https://handewith.cloudflareaccess.com` |
| `CF_ACCESS_AUD` | `c0a32fcdfc92e457efc5142804205eb6e06b547f2a6708e56ec5855ee2e9d048` |
| `STORYFORGE_ADMIN_EMAILS` | Your Google login email; comma-separated if more than one administrator |
| `STORYFORGE_RECORDINGS_DIR` | `/data/recordings` |

The team domain and AUD tag are identifiers, not secrets. No Google secret or
Cloudflare API token is needed. Keep your existing PostgreSQL variables, network,
and HTTP port mapping. Do not add the development authentication variables.

Apply these settings, then update/force-update `ghcr.io/mhandewith/storyforge:latest`.
The database migration runs automatically. If storage is not writable, startup
fails with a recording-storage message rather than accepting unsaved uploads.

## 3. Link the actors

1. Open **https://storyforge.handewith.family**, signing in with the administrator
   Google account you entered above.
2. In **Cast & characters**, enter each actor's Google email and choose **Save login**.
3. Assign each character to its actor using **Played by**.
4. Ensure Cloudflare Access allows each girl's specific Google account as well.
5. Each girl opens the same HTTPS address and signs in with her own account.
   Her actor studio opens automatically, showing only her assigned scenes.

Use the HTTPS address for both administration and recording. The direct LAN
address no longer grants access to workspace data, because it has no verified
Cloudflare identity. Health endpoints remain available to Docker.

An account without an actor mapping sees a setup message. An administrator can
also link their own email to Dad's actor to use **Recording studio**.

## 4. Try a performance

Choose a scene and read the highlighted words and direction. Press **Record**,
allow microphone access, and press **Stop recording**. Listening is optional;
**Save take** uploads it. Recording again never overwrites a saved take.

Unsaved recordings live only in the current browser page. Navigation prompts
before discarding them. **Download a copy** is available if an upload fails or
the login expires. Saving again after an uncertain upload uses the same upload
ID, preventing duplicate takes.

Previous takes are available under each line and **My saved takes**. The first
take becomes preferred; the star button can change that. Progress counts lines
with a saved take for the current script revision. Script edits preserve older
takes and mark them **Earlier script version**, with their original words.

Administrators can open **Review takes** to listen to everyone's submissions,
see progress, and change preferences. Recording is designed for Chrome on
Windows and Android. Real microphones and Android permission behavior should
be checked on the girls' devices; automated tests use Chromium's simulated mic.

Recordings stop automatically before five minutes; uploads are limited to 25 MB.
The backend checks the audio and stores the original bytes with a SHA-256 hash.
There is no ElevenLabs conversion, scene mixing, or automatic source deletion.

## Backups

Back up **both** PostgreSQL's `storyforge` database and the recording directory.
Audio is stored in the directory; the database holds ownership, script revisions,
take numbers, preferences, and audio metadata. Stop StoryForge while taking a
coordinated backup, then restart it. Keep the wiki's PostgreSQL container running.

```sh
docker exec postgresql15 pg_dump -U storyforge -d storyforge -Fc > /mnt/user/backups/storyforge.dump
```

Copy the recording directory to your backup location while StoryForge is stopped.
Never delete recordings just because a line, actor, scene, or script was removed;
those historical records are retained. A rare interrupted database commit can
leave an unreferenced source file; it is deliberately retained rather than
automatically deleted.
