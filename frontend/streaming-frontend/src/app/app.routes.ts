import { Routes } from '@angular/router';

export const appRoutes: Routes = [
  { path: '', pathMatch: 'full', redirectTo: 'app/vod' },
  { path: 'app', pathMatch: 'full', redirectTo: 'app/vod' },
  { path: 'app/vod', loadComponent: () => import('./pages/vod-page/vod-page.component').then(m => m.VodPageComponent) },
  { path: 'app/live', loadComponent: () => import('./pages/live-page/live-page.component').then(m => m.LivePageComponent) },
  { path: 'watch/vod/:id', loadComponent: () => import('./pages/watch-vod-page/watch-vod-page.component').then(m => m.WatchVodPageComponent) },
  { path: 'watch/live/:id', loadComponent: () => import('./pages/watch-live-page/watch-live-page.component').then(m => m.WatchLivePageComponent) },
  { path: '**', redirectTo: 'app/vod' },
];
