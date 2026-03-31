import { Routes } from '@angular/router';
import { VodPageComponent } from './pages/vod-page/vod-page.component';
import { WatchVodPageComponent } from './pages/watch-vod-page/watch-vod-page.component';
import { LivePageComponent } from './pages/live-page/live-page.component';
import { WatchLivePageComponent } from './pages/watch-live-page/watch-live-page.component';

export const appRoutes: Routes = [
  { path: '', pathMatch: 'full', redirectTo: 'app/vod' },
  { path: 'app', pathMatch: 'full', redirectTo: 'app/vod' },
  { path: 'app/vod', component: VodPageComponent },
  { path: 'app/live', component: LivePageComponent },
  { path: 'watch/vod/:id', component: WatchVodPageComponent },
  { path: 'watch/live/:id', component: WatchLivePageComponent },
  { path: '**', redirectTo: 'app/vod' },
];
