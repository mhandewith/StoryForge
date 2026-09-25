import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir:'./tests', fullyParallel:false, workers:1, retries:0,
  reporter:'list', use:{baseURL:'http://127.0.0.1:18088', trace:'retain-on-failure'},
});
