<script setup>
import {
  LocalParticipant,
  LocalTrackPublication,
  Participant,
  RemoteParticipant,
  RemoteTrack,
  RemoteTrackPublication,
  Room,
  RoomEvent,
  Track,
  VideoPresets,
} from 'livekit-client';
import {ref} from "vue";

const room = new Room()

const model = ref('1')
const token1 = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3MzM0MzA2NTIsImlzcyI6ImRldmtleSIsIm5iZiI6MTczMzQyNzA1Miwic3ViIjoicGFydGljaXBhbnRJZGVudGl0eTEiLCJ2aWRlbyI6eyJyb29tIjoibXlyb29tIiwicm9vbUpvaW4iOnRydWV9fQ.Z2TuFGJnK0eRs3bqzY5-KoZ7eQ5lmY85vZs5PkFY9zk'
const token2 = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3MzMzMTAyMTYsImlzcyI6ImRldmtleSIsIm5iZiI6MTczMzMwNjYxNiwic3ViIjoicGFydGljaXBhbnRJZGVudGl0eTIiLCJ2aWRlbyI6eyJyb29tIjoibXlyb29tIiwicm9vbUpvaW4iOnRydWV9fQ._gGzcdLA4BysCvRMfiwrQ1kF9jHHwHTfKYhu3DHNRSI'


async function connection() {
  const room = new Room()
  let token = ''
  if (model.value === '1') {
    token = token1
  } else {
    token = token2
  }

  await room.prepareConnection('http://localhost:7880', token);
  room
    .on(RoomEvent.TrackSubscribed, handleTrackSubscribed)
  //   .on(RoomEvent.ActiveSpeakersChanged, handleActiveSpeakerChange)
  //   .on(RoomEvent.LocalTrackUnpublished, handleLocalTrackUnpublished);
  await room.connect('ws://localhost:7880', token);
  console.log('connected to room', room.activeSpeakers);
  await room.localParticipant.setMicrophoneEnabled(true);

}

function handleTrackSubscribed(
    track,
    publication,
    participant,
) {
  if (track.kind === Track.Kind.Video || track.kind === Track.Kind.Audio) {
    // attach it to a new HTMLVideoElement or HTMLAudioElement
    attachTrack(track, participant)
  }
}

function attachTrack(track, participant) {
  const v = document.getElementById("remoteAudio");
  track.attach(v);
}

</script>

<template>
<input type="text" v-model="model">
<button @click="connection">Подключиться</button>
  <audio id="remoteAudio"></audio>
</template>

<style scoped>

</style>