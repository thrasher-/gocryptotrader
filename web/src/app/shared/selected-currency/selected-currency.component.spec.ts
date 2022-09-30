import { ComponentFixture, TestBed, waitForAsync  } from '@angular/core/testing';

import { SelectedCurrencyComponent } from './selected-currency.component';

describe('SelectedCurrencyComponent', () => {
  let component: SelectedCurrencyComponent;
  let fixture: ComponentFixture<SelectedCurrencyComponent>;

  beforeEach(waitForAsync(() => {
    TestBed.configureTestingModule({
      declarations: [ SelectedCurrencyComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(SelectedCurrencyComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
