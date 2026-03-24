import { ChangeDetectionStrategy, Component, output, signal } from '@angular/core';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

@Component({
  selector: 'app-upload-drawer',
  imports: [ReactiveFormsModule, MatFormFieldModule, MatInputModule, MatButtonModule, MatIconModule],
  templateUrl: './upload-drawer.component.html',
  styleUrl: './upload-drawer.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class UploadDrawerComponent {
  closed = output<void>();
  uploaded = output<{ title: string; description: string; file: File }>();

  file = signal<File | null>(null);

  form = new FormGroup({
    title: new FormControl('', { nonNullable: true, validators: [Validators.required] }),
    description: new FormControl('', { nonNullable: true }),
  });

  canSubmit(): boolean {
    return this.form.valid && !!this.file();
  }

  onFileSelected(event: Event): void {
    const target = event.target as HTMLInputElement;
    const selected = target.files?.[0] ?? null;
    this.file.set(selected);
  }

  openFilePicker(input: HTMLInputElement): void {
    input.click();
  }

  close(): void {
    this.closed.emit();
  }

  submit(): void {
    if (!this.canSubmit()) {
      this.form.markAllAsTouched();
      return;
    }

    const selectedFile = this.file();
    if (!selectedFile) {
      return;
    }

    this.uploaded.emit({
      title: this.form.controls.title.value.trim(),
      description: this.form.controls.description.value.trim(),
      file: selectedFile,
    });
  }

  resetForm(): void {
    this.form.reset({ title: '', description: '' });
    this.file.set(null);
  }
}
