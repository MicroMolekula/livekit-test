<script setup>
import {
  Room,
  RoomEvent,
  Track,
} from 'livekit-client';
import {ref} from "vue";

const result = ref('')
const token1 = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3MzQyNzQ2MzUsImlzcyI6ImRldmtleSIsIm5iZiI6MTczNDE4ODIzNSwic3ViIjoiaXZhbiIsInZpZGVvIjp7InJvb20iOiJteXJvb20iLCJyb29tSm9pbiI6dHJ1ZX19.nmcYBXN4SirzweF717I-XFUaBbO0w21aYcyJtxGfC2M'


async function connectionRTC() {
  const room = new Room()
  let token = token1

  await room.prepareConnection('http://localhost:7880', token);
  room
    .on(RoomEvent.TrackSubscribed, handleTrackSubscribed)
  await room.connect('ws://localhost:7880', token);
  console.log('connected to room', room.activeSpeakers);
  navigator.mediaDevices.getUserMedia({
    audio: {
      sampleRate: 48000
    }
  }).then((stream) => {
    const audioTrack = stream.getAudioTracks()[0];
    room.localParticipant.publishTrack(audioTrack);
  }).catch((error) => {
    console.error("Ошибка при доступе к микрофону:", error);
  });
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

console.log("Starting connection to WS")
let connectionEn = new WebSocket("ws://localhost:8088/en")

connectionEn.onerror = function(event) {
  console.log(event)
}

connectionEn.onopen = function(event) {
  console.log(event)
  console.log("Successfully connected to the echo WebSocket Server")
}

connectionEn.onmessage = function(event) {
  console.log(event.data)
  result.value = event.data
}
</script>

<template>
  <div class="flex flex-col h-screen">
    <!-- Header -->
    <header class="bg-green-600 text-white p-4 flex justify-between items-center">
      <div class="text-2xl font-semibold">Перевод в реальном времени</div>
      <div class="text-lg">Красиков Иван</div>
    </header>

    <!-- Main Content -->
    <div class="flex flex-col flex-1">
      <!-- Subtitles Section -->
      <div class="flex-1 flex items-center justify-center p-6 flex-col gap-1">
        <div class="bg-gray-800 text-white text-xl p-6 rounded-lg shadow-md w-full max-w-3xl">
          <p v-if="result" class="whitespace-pre-wrap">{{ result }}</p>
          <p v-else class="text-center text-gray-400">Место для субтитров</p>
        </div>
        <!-- Button under subtitles -->
        <button @click="connectionRTC" class="mt-4 px-6 py-2 bg-green-500 text-white rounded-lg hover:bg-green-600 focus:outline-none focus:ring-2 focus:ring-blue-500">
          Подключиться
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>

</style>