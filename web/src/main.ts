import { mount } from 'svelte';
import App from './App.svelte';
import './workspace.css';

mount(App, { target: document.getElementById('app')! });
