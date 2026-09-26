import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir:'./tests', fullyParallel:false, workers:1, retries:0,
  reporter:'list', use:{baseURL:'http://127.0.0.1:18088', trace:'retain-on-failure',extraHTTPHeaders:{Authorization:`Bearer ${process.env.STORYFORGE_DEV_AUTH_TOKEN}`,'X-StoryForge-Dev-Email':'admin@example.test'},launchOptions:{args:['--use-fake-device-for-media-stream','--use-fake-ui-for-media-stream']}},
});
