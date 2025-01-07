# High-Level Technical Design

This document is not complete yet. It is a work in progress.

## 1. Requirements

1. **Live-Only Streaming**
   - The broadcast is continuous.

2. **No Restart from the Beginning**
   - If the client closes and reopens the stream, it must jump into the live edge rather than replaying from segment #0.

3. **Synchronized Subtitles**
   - AI-based subtitles are generated for each audio chunk and served as a rolling feed so that clients only see real-time transcriptions.

## 2. High-Level Flow

1. **Live Audio Ingestion**
   - A radio or live feed is received and processed by FFmpeg in real time.

2. **Chunking and HLS Manifest Generation**
   - FFmpeg is run in a “live HLS” mode with a sliding window of segments.
   -  FFmpeg command produce new segments (N-second chunks) while maintaining a small rolling playlist (only the last M segments).

3. **Object Storage (MinIO)**
   - Generated `.m3u8` playlists and `.ts` segments are stored in a local directory, then synchronized or uploaded to MinIO.
   - Older segments are removed from the playlist and deleted as new ones arrive, maintaining a rolling window.
   - MinIO can employ a lifecycle policy or a custom cleanup script to automatically remove objects that are no longer referenced in the rolling HLS playlist. If the system only ever needs the last M segments, older `.vtt` files and `.ts` segments can be automatically expired after a short grace period. This approach prevents indefinite storage growth for an infinitely running stream.

4. **AI Transcription**
   - Each new segment (`<segment-id>.ts`) is passed to an AI service for speech-to-text.
   - The transcription is returned without timestamps; the system associates the text with the segment’s time range `[offset*<N-second>, (offset+1)*<N-second>]`.

5. **Subtitle Generation**
   - Partial `.vtt` files are created for each chunk, referencing only the N-second local offset for that chunk.
   - Partial WebVTT files use a simple naming convention (`subs0.vtt`, `subs1.vtt`, etc.) so that each file corresponds to a specific HLS segment (`segment0.ts`, `segment1.ts`, etc.). The subtitle playlist (`subs.m3u8`) references these .vtt files in the same order, inserting an `#EXTINF:N.0` and the filename. When the rolling window updates (for instance, only keeping the last M segments), older `.vtt` files are removed from `subs.m3u8` to align with the segments still present in the audio playlist.

6. **Serving the Stream**
   - A Go service supervises FFmpeg, updates MinIO with new segments, and provides a URL for the live HLS manifest.
   - The client’s HLS player fetches the updated `.m3u8` and only the segments within the rolling window, ensuring playback remains live.
   - Subtitles are loaded via a parallel HLS WebVTT playlist.

## 3. Detailed Responsibilities

### 3.1 FFmpeg

- Runs continuously against the live source.
- Produces a limited number of segments in an `.m3u8` playlist (for example, M segments at N seconds each).
- Deletes older segments automatically, ensuring a short rolling window.

### 3.2 Re-Stream Manager

- Supervises the FFmpeg process with the specified live HLS configuration.
- Uploads `.ts` segments and updated `.m3u8` playlists to Storage.
- Invokes the AI service for each newly created segment to obtain transcribed text.
- Generates `.vtt` subtitle segments tied to each chunk’s local N-second offset.
- Maintains a subtitle HLS playlist, removing entries that fall out of the rolling window.

### 3.3 Storage (MinIO)

- The storage for `.ts` segments, `.m3u8` files, and `.vtt` subtitle files.
- Removes or allows expiration of old segments once they slide out of the defined live window.

### 3.4 Client

- Uses an HLS player ([hls.js](https://github.com/video-dev/hls.js) in a browser) to fetch the live `.m3u8`.
- Receives only the newest segments defined by the rolling playlist, ensuring playback from the live edge.
- Loads subtitles from a corresponding `.m3u8` file containing partial `.vtt` segments.

## 4. Ensuring a Live-Only Experience

- An HLS playlist configured with a short [-hls_list_size](https://ffmpeg.org/ffmpeg-formats.html#:~:text=time%20has%20passed.-,hls_list_size,-size) and [-hls_flags](https://ffmpeg.org/ffmpeg-formats.html#:~:text=streams%20in%20subdirectories.-,hls_flags,-flags) `delete_segments` will remove older segments and disallow scrubbing to the beginning.
- When a client rejoins, the manifest references only the current window of segments, guaranteeing playback at the live position.

